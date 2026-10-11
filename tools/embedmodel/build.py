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
CJK_BUDGET = int(sys.argv[4]) if len(sys.argv) > 4 else 70000
ASCII_BUDGET = int(sys.argv[5]) if len(sys.argv) > 5 else 30000
DIMS = int(sys.argv[6]) if len(sys.argv) > 6 else 128

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


def is_cjk(ch):
    name = unicodedata.name(ch, "")
    return any(k in name for k in ("CJK", "HIRAGANA", "KATAKANA", "IDEOGRAPHIC"))


cands = [i for i, (p, _) in enumerate(vocab) if wanted(p)]
# Single characters always stay, so that any text can still be cut into
# tokens. Of longer pieces, the most frequent (by Unigram score) of each
# kind within its budget: Chinese and Japanese, then English; the pieces
# of other languages written in Latin letters go.
must, cjk, ascii_ = set(), [], []
for i in cands:
    core = vocab[i][0].replace("\u2581", "")
    if len(core) <= 1:
        must.add(i)
    elif any(is_cjk(ch) for ch in core):
        cjk.append(i)
    elif all(ord(ch) < 0x80 for ch in core):
        ascii_.append(i)
print("pieces: single", len(must), "cjk", len(cjk), "ascii", len(ascii_), "other", len(cands) - len(must) - len(cjk) - len(ascii_))
cjk.sort(key=lambda i: -vocab[i][1])
ascii_.sort(key=lambda i: -vocab[i][1])
keep = sorted(must | set(cjk[:CJK_BUDGET]) | set(ascii_[:ASCII_BUDGET]) | {unk})
print("kept:", len(keep))

small = Tokenizer.from_str(tok.to_str())
small.model = Unigram([vocab[i] for i in keep], keep.index(unk), False)
small_E = E[keep]
# Fewer dimensions: the principal components of the vectors kept.
if DIMS < small_E.shape[1]:
    mu = small_E.mean(axis=0)
    _, sv, vt = np.linalg.svd(small_E - mu, full_matrices=False)
    var = (sv ** 2) / (sv ** 2).sum()
    print(f"PCA to {DIMS}: keeps {var[:DIMS].sum():.3f} of the variance")
    small_E = ((small_E - mu) @ vt[:DIMS].T).astype(np.float32)


def encode(t, emb, text):
    enc = t.encode(text, add_special_tokens=False)
    ids = [i for i, tk in zip(enc.ids, enc.tokens) if tk != "<unk>"][:512]
    if not ids:
        return np.zeros(emb.shape[1], dtype=np.float32), enc.tokens
    v = emb[ids].mean(axis=0)
    return v / (np.linalg.norm(v) or 1), enc.tokens


# What the normalizer's precompiled charsmap makes of each character it
# changes: the charsmap alone, the rest of the normalizer being rules the
# reader runs itself.
def find_precompiled(n):
    if isinstance(n, dict):
        if n.get("type") == "Precompiled":
            return n
        for v in n.values():
            if (f := find_precompiled(v)) is not None:
                return f
    if isinstance(n, list):
        for v in n:
            if (f := find_precompiled(v)) is not None:
                return f
    return None


pre_only = dict(tj)
pre_only["normalizer"] = find_precompiled(tj["normalizer"])
charsmap = Tokenizer.from_str(json.dumps(pre_only)).normalizer
charmap = {}
for o in list(range(0x0, 0xA000)) + list(range(0xF900, 0x10000)):
    ch = chr(o)
    if unicodedata.category(ch) in ("Cs", "Co", "Cn"):
        continue
    out = charsmap.normalize_str(ch)
    if out != ch:
        charmap[ch] = out
print("charmap entries:", len(charmap))

header = {
    "name": NAME,
    "kind": "Unigram",
    "dim": int(small_E.shape[1]),
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
    for k, i in enumerate(keep):
        piece, score = vocab[i]
        v = small_E[k]
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
ref = []
for s_ in samples:
    v, tokens = encode(small, small_E, s_)
    print(f"{s_!r}  {tokens[:16]}")
    ref.append({"text": s_, "normalized": tok.normalizer.normalize_str(s_), "tokens": tokens, "vector": [float(x) for x in v]})

# Do the trimmed model's similarities agree with the full model's? And
# does each query find its document?
pairs = [("数据库变慢怎么办", "SQL 优化：联合索引遵循最左前缀"), ("想吃家常菜", "番茄炒蛋的做法"),
         ("出国玩", "下周去东京旅行，记得订机票"), ("前端框架的副作用", "React useEffect 的依赖数组"),
         ("并发编程", "Go 语言并发：goroutine 和 channel"), ("AI 学习", "机器学习入门资料"),
         ("缓存失效导致数据库压力", "缓存雪崩和缓存击穿的区别"), ("写需求", "产品需求文档模板"),
         ("开会时间调整", "周会改到周四下午三点"), ("手机测评", "iPhone 16 Pro Max 评测"),
         ("深度学习教程", "神经网络从零开始：反向传播详解"), ("理财", "指数基金定投的三个原则"),
         ("健身计划", "每周三次力量训练，深蹲硬拉卧推"), ("写简历", "求职材料：个人经历与项目经验怎么写"),
         ("Python 爬虫", "用 requests 和 BeautifulSoup 抓取网页"), ("团队管理", "一对一沟通的技巧"),
         ("database index", "SQL 优化：联合索引遵循最左前缀"), ("travel to Japan", "下周去东京旅行，记得订机票")]
docs = sorted({d for _, d in pairs})
texts = [q for q, _ in pairs] + docs


def sims(t, emb):
    V = np.stack([encode(t, emb, x)[0] for x in texts])
    return V @ V.T


full_S, small_S = sims(tok, E), sims(small, small_E)
iu = np.triu_indices(len(texts), 1)
print(f"similarities: correlation trimmed vs full {np.corrcoef(full_S[iu], small_S[iu])[0, 1]:.3f}")
for name, t, emb in (("full", tok, E), ("trimmed", small, small_E)):
    D = np.stack([encode(t, emb, d)[0] for d in docs])
    right = 0
    for q, d in pairs:
        best = docs[int(np.argmax(D @ encode(t, emb, q)[0]))]
        right += best == d
        if best != d:
            print(f"  {name}: {q} -> {best}")
    print(f"{name}: {right}/{len(pairs)} queries find their document")

with open(REF, "w") as f:
    json.dump({"name": NAME, "samples": ref}, f, ensure_ascii=False)
print("wrote", OUT, "and", REF)
