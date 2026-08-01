#!/bin/sh
# PostToolUse hook — HubScope 令牌纪律检查(补 impeccable detect.mjs 盲区)。
# 页面/组件只消费 semantics.css 语义令牌:禁硬编码色值(hex/rgb/rgba)、
# 禁书写 --el-* 变量(全仓唯一允许处是 web/src/styles/ep-theme.css)。
# 豁免(与 ui-guidelines §2 修改纪律一致):
#   - web/src/styles/*            令牌定义层,字面量是本职工作
#   - components/BrandMark.vue    渐变 stop 表现属性(snapdom 兜底,§2b 登记)
#   - utils/chartColors.ts        ECharts 色板 JS 镜像(§3 登记)
#   - utils/vendorIcon.ts vendorIcon.test.ts         供应商品牌色外部资产(§5 供应商图标条登记,BrandMark 同类)
#   - --el-card-padding           密度档既定机制(§2 间距条目)
# 只报不拦(exit 0),findings 经 additionalContext 注入会话。
set -u

file=$(sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
project_dir=${CLAUDE_PROJECT_DIR:-}

case "$file" in
  "$project_dir"/web/src/*.vue|"$project_dir"/web/src/*.ts|"$project_dir"/web/src/*.css) ;;
  *) exit 0 ;;
esac

# 豁免文件:令牌定义层与三处登记例外
case "$file" in
  */web/src/styles/*|*/web/src/components/BrandMark.vue|*/web/src/utils/chartColors.ts|*/web/src/utils/vendorIcon.ts|*/web/src/utils/vendorIcon.test.ts) exit 0 ;;
esac

[ -f "$file" ] || exit 0

# 去注释行与锚点引用(href="#..." 非色值),减少误报;再剥 issue 引用
# 「GH #NNN」——块注释续行(无 * 前缀)里的票号会撞 3 位 hex 字面量
# (如 GH #113 → "#113" 命中 3-hex 模式,GH #121/#118 等一大批误报)。
filtered=$(grep -vE '^[[:space:]]*(//|/\*|\*|<!--)' "$file" | sed 's/href="#[^"]*"//g; s/GH #[0-9][0-9]*//g' || true)

findings=""
hex_hits=$(printf '%s\n' "$filtered" | grep -nE '#([0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{4}|[0-9a-fA-F]{3})([^0-9a-zA-Z]|$)|rgba?\(' | head -5 || true)
if [ -n "$hex_hits" ]; then
  findings="${findings}硬编码色值:
${hex_hits}
"
fi

# 出现级豁免:逐个 --el-* 变量过滤,只放行 --el-card-padding(避免同行共存误杀)
el_hits=$(printf '%s\n' "$filtered" | grep -noE -- '--el-[a-z0-9-]+' | grep -v -- '--el-card-padding$' | head -5 || true)
if [ -n "$el_hits" ]; then
  findings="${findings}--el-* 书写:
${el_hits}
"
fi

if [ -n "$findings" ]; then
  msg="[check-tokens] $(basename "$file") 命中令牌纪律(页面/组件只消费 semantics.css 语义令牌):
${findings}如为登记豁免(--el-card-padding/BrandMark/chartColors 之外的新例外),需先在规范登记再加豁免。"
  esc=$(printf '%s' "$msg" | sed 's/\\/\\\\/g; s/"/\\"/g' | sed ':a;N;$!ba;s/\n/\\n/g')
  printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"%s"}}' "$esc"
fi
exit 0
