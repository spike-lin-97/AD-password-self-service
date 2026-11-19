#!/bin/bash
# 覆盖 pkg/feishu/feishu.go 并做语法校验
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
DEST="$DIR/pkg/feishu/feishu.go"

if [ ! -f "$DIR/feishu.go" ]; then
    echo "❌ 未找到同目录下的 feishu.go，请把 feishu.go 放到 $DIR 下"
    exit 1
fi

echo "[1/4] 覆盖 $DEST"
cp "$DIR/feishu.go" "$DEST"

# [2/4] 校验括号配对（Go 对 ( ) { } 数量平衡有严格要求）
echo "[2/4] 检查括号配对"
PARENS=$(tr -cd '()' < "$DEST" | wc -c)
BRACES=$(tr -cd '{}' < "$DEST" | wc -c)
if [ $((PARENS % 2)) -ne 0 ] || [ $((BRACES % 2)) -ne 0 ]; then
    echo "❌ 括号未配对：() 数量=$PARENS (应为偶数)，{} 数量=$BRACES (应为偶数)"
    exit 1
fi
echo "     () 数量=$PARENS, {} 数量=$BRACES ✓"

# [3/4] 只检查「代码区」的全角字符（去掉 // 行注释 和 "..." 字符串字面量）
echo "[3/4] 检查代码区全角字符（忽略注释与字符串）"
python3 - "$DEST" <<'PY'
import re, sys
path = sys.argv[1]
with open(path, encoding='utf-8') as f:
    lines = f.readlines()

# 逐行去除：行注释 // ...   和   "..." 字符串字面量
cleaned = []
for ln in lines:
    s = ln
    # 去掉双引号字符串（含转义 \"）
    s = re.sub(r'"(\\.|[^"\\])*"', '""', s)
    # 去掉反引号字符串 `...`
    s = re.sub(r'`[^`]*`', '``', s)
    # 去掉 // 行注释（注意：要先去掉字符串，避免 http:// 被误当注释）
    s = re.sub(r'//.*$', '', s)
    cleaned.append(s)

text = "\n".join(cleaned)
# 常见会导致 Go 语法错误的全角符号
bad = [
    ('（', '('), '）', '（',
    '）', ')', '（', ')',
    '，', ',', '；', ';',
    '：', ':', '“', '"',
    '”', '"', '‘', "'",
    '’', "'", '　', ' ',
]
# 用集合检测 CJK 全角标点
import unicodedata
fullwidth = set()
for ch in text:
    if ord(ch) > 0x2000 and unicodedata.category(ch).startswith('P'):
        # 排除已经在 ASCII 范围的半角标点
        if ord(ch) < 128:
            continue
        fullwidth.add(ch)

if fullwidth:
    print("❌ 代码区检测到全角标点字符：", "".join(sorted(fullwidth)))
    print("   请将其替换为半角：，;: 等")
    sys.exit(1)
print("   代码区无全角标点 ✓")
PY

# [4/4] 用 go vet 做真正的语法检查
echo "[4/4] go vet 校验（权威）"
cd "$DIR"
go vet ./pkg/feishu/...
echo "✅ go vet 通过，feishu.go 语法正确"
echo ""
echo "下一步："
echo "  go build ./..."
echo "  make run"

