"""Builds MindMap's built-in embedding model from a Model2Vec model.

Model2Vec distills a sentence transformer (here potion-multilingual-128M,
distilled from BAAI/bge-m3) into a table of token vectors: a text's vector
is the mean of its tokens' vectors. This keeps the tokens of Chinese,
Japanese and English (the app's languages), and of those the most frequent,
quantizes each vector to int8 with a scale of its own, and writes them
with the tokenizer's vocabulary in the format internal/embed reads:

    "MMEMB1"  u32 header length  header JSON
    u32 token count
    then for each token: u16 byte length, the token (UTF-8),
    f32 Unigram score, f32 scale, dim × int8

The header carries what the normalizer does to each character, since the
reader cannot run its precompiled charsmap.

It also writes reference.json: sample texts with their normalized form,
the tokens and vector of the trimmed model (which internal/embed must
reproduce), and how alike the trimmed and the full model's vectors are.

    python build.py MODEL OUT.bin reference.json [TOKENS]
"""

import json
import struct
import sys
import unicodedata

import numpy as np
from model2vec import StaticModel
from tokenizers import Tokenizer
from tokenizers.models import Unigram

NAME = sys.argv[1] if len(sys.argv) > 1 else "minishlab/potion-multilingual-128M"
OUT = sys.argv[2] if len(sys.argv) > 2 else "mindmap-embed.bin"
REF = sys.argv[3] if len(sys.argv) > 3 else "reference.json"
KEEP = int(sys.argv[4]) if len(sys.argv) > 4 else 64000

m = StaticModel.from_pretrained(NAME)
tok = m.tokenizer
tj = json.loads(tok.to_str())
model = tj["model"]
if model["type"] != "Unigram":
    sys.exit("unsupported tokenizer " + model["type"])
E = np.asarray(m.embedding, dtype=np.float32)
vocab = [(p, float(s)) for p, s in model["vocab"]]
unk = model["unk_id"]
print("vocab:", len(vocab), "dim:", E.shape[1], "unk:", unk)


def strip_charsmap(n):
    if isinstance(n, dict):
        return {k: strip_charsmap(v) for k, v in n.items() if k != "precompiled_charsmap"}
    if isinstance(n, list):
        return [strip_charsmap(v) for v in n]
    return n


print("normalizer:", json.dumps(strip_charsmap(tj.get("normalizer")), ensure_ascii=False))
print("pre_tokenizer:", json.dumps(tj.get("pre_tokenizer"), ensure_ascii=False))


def wanted(piece):
    """Tokens of Chinese, Japanese, English, digits and punctuation."""
    s = piece.replace("▁", "")
    for ch in s:
        if ord(ch) < 0x250:  # ASCII and Latin
            continue
        name = unicodedata.name(ch, "")
        if any(k in name for k in ("CJK", "HIRAGANA", "KATAKANA", "FULLWIDTH", "HALFWIDTH", "IDEOGRAPHIC")):
            continue
        if unicodedata.category(ch)[0] in "PSZN":
            continue
        return False
    return True


cands = [i for i, (p, _) in enumerate(vocab) if wanted(p)]
# Single characters always stay, so that any text can still be cut into
# tokens; then the most frequent pieces, by their Unigram scores.
must = {i for i in cands if len(vocab[i][0].replace("▁", "")) <= 1}
rest = sorted((i for i in cands if i not in must), key=lambda i: -vocab[i][1])
keep = sorted(must | set(rest[: max(0, KEEP - len(must))]) | {unk})
print("wanted:", len(cands), "kept:", len(keep), "single characters:", len(must))

small = Tokenizer.from_str(tok.to_str())
small.model = Unigram([vocab[i] for i in keep], keep.index(unk), False)
small_E = E[keep]


def encode(t, emb, text):
    enc = t.encode(text, add_special_tokens=False)
    ids = [i for i, tk in zip(enc.ids, enc.tokens) if tk != "<unk>"]
    if not ids:
        return np.zeros(emb.shape[1], dtype=np.float32), enc.tokens
    v = emb[ids].mean(axis=0)
    return v / (np.linalg.norm(v) or 1), enc.tokens


# What the normalizer makes of each character it changes.
charmap = {}
for o in list(range(0x20, 0xA000)) + list(range(0xF900, 0x10000)):
    ch = chr(o)
    if unicodedata.category(ch) in ("Cs", "Co", "Cn"):
        continue
    out = tok.normalizer.normalize_str(ch)
    if out != ch:
        charmap[ch] = out
print("charmap entries:", len(charmap))

header = {
    "name": NAME,
    "kind": "Unigram",
    "dim": int(E.shape[1]),
    "unk": keep.index(unk),
    "normalizer": strip_charsmap(tj.get("normalizer")),
    "pre_tokenizer": tj.get("pre_tokenizer"),
    "charmap": charmap,
    "normalize": True,
}
with open(OUT, "wb") as f:
    h = json.dumps(header, ensure_ascii=False).encode()
    f.write(b"MMEMB1")
    f.write(struct.pack("<I", len(h)))
    f.write(h)
    f.write(struct.pack("<I", len(keep)))
    for i in keep:
        piece, score = vocab[i]
        v = E[i]
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
    "（括号）【方括号】《书名号》「引号」…", "Ｃ＋＋ ｖｓ Ｒｕｓｔ", "第①章 ½ ㎡ ℃",
]
full = m.encode(samples)
ref = []
for s, fv in zip(samples, full):
    v, tokens = encode(small, small_E, s)
    fv = fv / (np.linalg.norm(fv) or 1)
    cos = float(v @ fv)
    print(f"trimmed vs full {cos:.4f}  {s!r}  {tokens[:14]}")
    ref.append({"text": s, "normalized": tok.normalizer.normalize_str(s),
                "pretokenized": [w for w, _ in tok.pre_tokenizer.pre_tokenize_str(tok.normalizer.normalize_str(s))],
                "tokens": tokens, "vector": [float(x) for x in v], "full": cos})

# Retrieval: does the trimmed model still put each query nearest its
# document, as the full one does?
pairs = [("数据库变慢怎么办", "SQL 优化：联合索引遵循最左前缀"), ("想吃家常菜", "番茄炒蛋的做法"),
         ("出国玩", "下周去东京旅行，记得订机票"), ("前端框架的副作用", "React useEffect 的依赖数组"),
         ("并发编程", "Go 语言并发：goroutine 和 channel"), ("AI 学习", "机器学习入门资料"),
         ("缓存失效导致数据库压力", "缓存雪崩和缓存击穿的区别"), ("写需求", "产品需求文档模板")]
docs = [d for _, d in pairs]
for name, t, emb in (("full", tok, E), ("trimmed", small, small_E)):
    D = np.stack([encode(t, emb, d)[0] for d in docs])
    right = 0
    for q, d in pairs:
        qv = encode(t, emb, q)[0]
        best = docs[int(np.argmax(D @ qv))]
        right += best == d
        print(f"  {name}: {q} -> {best}")
    print(f"{name}: {right}/{len(pairs)} queries find their document")

with open(REF, "w") as f:
    json.dump({"name": NAME, "samples": ref}, f, ensure_ascii=False)
print("wrote", OUT, "and", REF)
