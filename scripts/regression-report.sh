#!/usr/bin/env bash
# 回归测试 report 管道：跑全量单元/集成测试（带覆盖率），把结果写入
# output/regression-report.txt 并纳入 git。report 记录 PASS/FAIL 总判定、去除耗时的
# 包级结果（保证可复现），以及函数级覆盖率快照。临时 coverage.out 被 .gitignore 排除。
#
# 用法: ./scripts/regression-report.sh   （CI 与本地收口统一调用）
# 退出码 = go test 的退出码（红色回归会同时写进 report 并以非零退出暴露）。
set -uo pipefail
cd "$(dirname "$0")/.."

mkdir -p output
COVER=output/coverage.out
REPORT=output/regression-report.txt

test_out=$(go test ./... -coverprofile="$COVER" 2>&1)
code=$?

{
	echo "JapanDigitalPostService 回归测试 report"
	echo "command: go test ./... -coverprofile=output/coverage.out"
	if [ "$code" -eq 0 ]; then
		echo "RESULT: PASS"
	else
		echo "RESULT: FAIL (exit $code)"
	fi
	echo
	echo "== 包级结果（去除耗时/缓存标记并排序，保证可复现）=="
	# 去掉每包耗时（如 \t0.041s，可能在行中）与 (cached)，并按包名排序——避免 report 因
	# 计时漂移或包构建完成顺序变化产生噪声 diff。覆盖率百分比保留（随代码变化属预期信号）。
	echo "$test_out" | sed -E 's/[[:space:]]+[0-9]+\.[0-9]+s//g; s/[[:space:]]*\(cached\)//g' | LC_ALL=C sort
	echo
	if [ -f "$COVER" ]; then
		echo "== 覆盖率（go tool cover -func）=="
		go tool cover -func="$COVER"
	fi
} >"$REPORT"

rm -f "$COVER"
echo "wrote $REPORT (go test exit $code)"
exit "$code"
