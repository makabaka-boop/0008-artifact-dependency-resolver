#!/usr/bin/env bash
# 冒烟测试：真实启动服务、请求健康检查、走通核心流程并清理进程与临时数据。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMPDIR="$(mktemp -d)"
PORT="${SMOKE_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"
DB_FILE="${TMPDIR}/smoke.db"
SERVER_PID=""

cleanup() {
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMPDIR}"
}
trap cleanup EXIT

# 构建并启动服务。
cd "${ROOT}"
go build -o "${TMPDIR}/server" ./cmd/server

mkdir -p "${TMPDIR}/data"
LISTEN_ADDR=":${PORT}" DB_PATH="${DB_FILE}" "${TMPDIR}/server" &
SERVER_PID=$!

# 轮询健康检查。
ok=0
for _ in $(seq 1 40); do
  if curl -fsS "${BASE}/healthz" >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep 0.25
done
if [[ "${ok}" != "1" ]]; then
  echo "FAIL: healthz not reachable" >&2
  exit 1
fi
echo "healthz OK"

health="$(curl -fsS "${BASE}/healthz")"
echo "${health}" | grep -q '"status":"ok"'
echo "healthz body: ${health}"

# 创建制品与版本，发布并声明依赖。
curl -fsS -X POST "${BASE}/artifacts" -H 'Content-Type: application/json' \
  -d '{"name":"liba","description":"library a"}' >/dev/null
curl -fsS -X POST "${BASE}/artifacts" -H 'Content-Type: application/json' \
  -d '{"name":"app","description":"application"}' >/dev/null

curl -fsS -X POST "${BASE}/artifacts/liba/versions" -H 'Content-Type: application/json' \
  -d '{"version":"1.0.0"}' >/dev/null
curl -fsS -X POST "${BASE}/artifacts/liba/versions" -H 'Content-Type: application/json' \
  -d '{"version":"1.2.0"}' >/dev/null
curl -fsS -X POST "${BASE}/artifacts/app/versions" -H 'Content-Type: application/json' \
  -d '{"version":"1.0.0"}' >/dev/null

curl -fsS -X PUT "${BASE}/artifacts/app/versions/1.0.0/dependencies" -H 'Content-Type: application/json' \
  -d '{"dependencies":[{"name":"liba","constraint":"^1.0.0"}]}' >/dev/null

curl -fsS -X POST "${BASE}/artifacts/liba/versions/1.0.0/publish" >/dev/null
curl -fsS -X POST "${BASE}/artifacts/liba/versions/1.2.0/publish" >/dev/null
curl -fsS -X POST "${BASE}/artifacts/app/versions/1.0.0/publish" >/dev/null

# 解析。
resolve="$(curl -fsS -X POST "${BASE}/resolve" -H 'Content-Type: application/json' \
  -d '{"manifest":[{"name":"app","constraint":"^1.0.0"}]}')"
echo "resolve: ${resolve}"
echo "${resolve}" | grep -q '"status":"succeeded"'
echo "${resolve}" | grep -q '"version":"1.2.0"'

# 版本比较。
cmp="$(curl -fsS "${BASE}/compare?left=1.2.3&right=1.2.4")"
echo "compare: ${cmp}"
echo "${cmp}" | grep -q '"result":-1'

echo "SMOKE TEST PASSED"
