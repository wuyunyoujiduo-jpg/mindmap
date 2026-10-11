"""Builds MindMap's built-in embedding model from a Model2Vec model.

Model2Vec distills a sentence transformer (here potion-multilingual-128M,
distilled from BAAI/bge-m3) into a table of token vectors: a text's vector
is the mean of its tokens' vectors. This keeps the tokens of Chinese,
Japanese and English (the app's languages), quantizes each vector to int8
with a scale of its own, and writes them with the tokenizer's vocabulary
in the format internal/embed reads:

    "MMEMB1"  u32 header length  header JSON
    then for each token: u16 byte length, the token (UTF-8),
    f32 score (Unigram), f32 scale, dim × int8

It also writes reference.json: sample texts with the tokens and vectors
model2vec gives them, which internal/embed's tests compare against.
"""

import json
import struct
import sys
import unicodedata

import numpy as np
from model2vec import StaticModel

NAME = sys.argv[1] if len(sys.argv) > 1 else "minishlab/potion-multilingual-128M"
OUT = sys.argv[2] if len(sys.argv) > 2 else "mindmap-embed.bin"
REF = sys.argv[3] if len(sys.argv) > 3 else "reference.json"

m = StaticModel.from_pretrained(NAME)
tok = m.tokenizer
tj = json.loads(tok.to_str())
model = tj["model"]
print("model type:", model["type"])
print("normalizer:", json.dumps(tj.get("normalizer"))[:600])
print("pre_tokenizer:", json.dumps(tj.get("pre_tokenizer"))[:600])
print("post_processor:", json.dumps(tj.get("post_processor"))[:300])
print("attributes:", [a for a in vars(m)])
print("embedding:", m.embedding.shape, m.embedding.dtype)
print("config:", getattr(m, "config", None))

E = np.asarray(m.embedding, dtype=np.float32)
mapping = getattr(m, "token_mapping", None)
weights = getattr(m, "weights", None)
print("token_mapping:", None if mapping is None else np.asarray(mapping).shape,
      "weights:", None if weights is None else np.asarray(weights).shape)


def token_vector(i):
    v = E[int(mapping[i])] if mapping is not None else E[i]
    if weights is not None:
        v = v * float(weights[i])
    return v


kind = model["type"]
if kind == "Unigram":
    vocab = [(p, float(s)) for p, s in model["vocab"]]
    unk = model.get("unk_id")
elif kind == "WordPiece":
    inv = sorted(model["vocab"].items(), key=lambda kv: kv[1])
    vocab = [(p, 0.0) for p, _ in inv]
    unk = model["vocab"].get(model.get("unk_token", "[UNK]"))
else:
    sys.exit("unsupported tokenizer " + kind)
print("vocab:", len(vocab), "unk:", unk)


def wanted(piece):
    """Keeps tokens of Chinese, Japanese, English, digits and punctuation."""
    s = piece.replace("▁", "").replace("##", "")
    if s == "":
        return True
    for ch in s:
        o = ord(ch)
        if o < 0x250:  # ASCII and Latin
            continue
        name = unicodedata.name(ch, "")
        if any(k in name for k in ("CJK", "HIRAGANA", "KATAKANA", "FULLWIDTH", "HALFWIDTH", "IDEOGRAPHIC")):
            continue
        if unicodedata.category(ch)[0] in "PSZN":  # punctuation, symbols, spaces, numbers
            continue
        return False
    return True


keep = [i for i, (p, _) in enumerate(vocab) if wanted(p) or i == unk]
print("kept:", len(keep), "of", len(vocab))

dim = E.shape[1]
header = {
    "name": NAME,
    "kind": kind,
    "dim": dim,
    "unk": keep.index(unk) if unk is not None and unk in keep else -1,
    "normalizer": tj.get("normalizer"),
    "pre_tokenizer": tj.get("pre_tokenizer"),
    "decoder": tj.get("decoder"),
    "model": {k: v for k, v in model.items() if k != "vocab"},
    "normalize": bool(getattr(m, "normalize", True)),
}
# The precompiled charsmap is large and the reader does NFKC instead.
if header["normalizer"] and "normalizers" in header["normalizer"]:
    for n in header["normalizer"]["normalizers"]:
        n.pop("precompiled_charsmap", None)
elif header["normalizer"]:
    header["normalizer"].pop("precompiled_charsmap", None)

with open(OUT, "wb") as f:
    h = json.dumps(header, ensure_ascii=False).encode()
    f.write(b"MMEMB1")
    f.write(struct.pack("<I", len(h)))
    f.write(h)
    f.write(struct.pack("<I", len(keep)))
    for i in keep:
        piece, score = vocab[i]
        v = token_vector(i).astype(np.float32)
        scale = float(np.abs(v).max()) / 127 or 1.0
        q = np.clip(np.round(v / scale), -127, 127).astype(np.int8)
        b = piece.encode()
        f.write(struct.pack("<H", len(b)))
        f.write(b)
        f.write(struct.pack("<ff", score, scale))
        f.write(q.tobytes())

samples = [
    "数据库变慢怎么办", "SQL 优化：联合索引遵循最左前缀", "番茄炒蛋的做法", "下周去东京旅行，记得订机票",
    "React useEffect 的依赖数组", "How to speed up a slow PostgreSQL query", "缓存雪崩和缓存击穿的区别",
    "产品需求文档模板", "周会改到周四下午三点", "机器学习入门资料", "Go 语言并发：goroutine 和 channel",
    "Hello, World!", "  多个   空格\t和制表符 ", "全角ＡＢＣ１２３和半角abc123", "東京タワーに行きたい",
    "iPhone 16 Pro Max 评测", "https://example.com/a?b=1", "e-mail: test@example.com", "😀 表情符号",
]
ref = []
vecs = m.encode(samples)
for s, v in zip(samples, vecs):
    ids = tok.encode(s, add_special_tokens=False).ids
    ref.append({"text": s, "tokens": [vocab[i][0] for i in ids], "vector": [float(x) for x in v]})
# Our mean of token vectors, for the full vocabulary, must match encode().
for r, s in zip(ref, samples):
    ids = [i for i in tok.encode(s, add_special_tokens=False).ids if i != unk]
    if not ids:
        continue
    mine = np.mean([token_vector(i) for i in ids], axis=0)
    mine = mine / (np.linalg.norm(mine) or 1)
    v = np.asarray(r["vector"])
    cos = float(mine @ (v / (np.linalg.norm(v) or 1)))
    print(f"self-check {cos:.4f}  {s!r}  {r['tokens'][:12]}")
with open(REF, "w") as f:
    json.dump({"name": NAME, "samples": ref}, f, ensure_ascii=False)
print("wrote", OUT, "and", REF)
