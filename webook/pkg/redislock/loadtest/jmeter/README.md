# redislock JMeter 压测

JMeter 驱动 HTTP，redislock 是 Go 库 —— 中间用 **loadserver** 把锁操作暴露成 HTTP 端点：

```
JMeter 线程 ──HTTP──> loadserver ──> redislock（真实 Go 代码）──> 真 Redis
                          └── 服务端跟踪互斥/fencing 不变量（客户端看不到）→ /stats
```

**关键**：JMeter 只测吞吐/延迟/错误率；「同一 key 是否出现过 >1 持有者」这类正确性铁证由 loadserver
在服务端用共享计数器跟踪，跑完看 `/stats` 的 `mutexViolations` / `fenceMonotonicBreaks` 必须为 **0**。

## 1. 启动 loadserver

```powershell
$env:REDISLOCK_REDIS_ADDR="127.0.0.1:6379"   # 缺省即此值
$env:REDISLOCK_REDIS_PASS="13520"            # 本地 dev 密码
go run ./pkg/redislock/loadtest/cmd/loadserver -addr :8099
```

## 2. 跑 JMeter（CLI / 非 GUI，压测必用非 GUI）

```bash
# 强竞争 + watchdog + fencing：64 线程、单 key、hold 20ms、跑 60s
jmeter -n -t redislock.jmx -l results.jtl \
  -Jthreads=64 -Jrampup=5 -Jduration=60 \
  -Jkey=jmeter:lock -Jhold=20 -Jlease=0s -Jfencing=true

# 生成 HTML 报告（吞吐/延迟分位/错误率）
jmeter -g results.jtl -o report/
```

### 压 watchdog（续约 + 丢锁检测）

默认 `hold`(20ms) 远小于续约间隔，压不到 watchdog。要触发续约/丢锁，用**短 watchdog 超时 + 长 hold**：

```bash
# watchdog 每 ~166ms 续约一次，每次持有(2s)跨十余次续约
jmeter -n -t redislock.jmx -l wd.jtl \
  -Jthreads=16 -Jduration=60 -Jkey=jmeter:lock -Jlease=0s -Jwatchdog=500ms -Jhold=2000
```

- **续约保活**：`/stats` 的 `mutexViolations=0` —— 2s 持有期远超 500ms 租约，靠 watchdog 续约才没被别人抢走。
- **丢锁检测**：压测中途 `redis-cli -a <pass> shutdown nosave`（或断网）几秒，`/metrics` 的
  `webook_lock_watchdog_lost_total` 会涨（续约失败超租约 → 视同丢锁、goroutine 退出），Redis 恢复后继续。

### 压公平锁（FIFO 排队）

```bash
# 32 线程抢单 key，fair 排队 + wait=2s；公平模式下几乎无 busy（都排队等），延迟↑吞吐↓换 FIFO 无饿死
jmeter -n -t redislock.jmx -l fair.jtl \
  -Jthreads=32 -Jduration=60 -Jkey=jmeter:fair -Jlease=3s -Jfair=true -Jwait=2s -Jfencing=false -Jhold=5
```

- **互斥仍成立**：`/stats` 的 `mutexViolations=0`（公平只改获取顺序，不改"同时只一个持有者"）。
- **FIFO 顺序**由库的单测 / 集成测 / `loadtest.Config.Run` 校验（HTTP 无状态难判定入队序）；JMeter 这里验公平下的吞吐/延迟/互斥。
- `fair=true` **必须配 `fencing=false`**（二者组合库返回 `ErrFairFencingUnsupported`），且 `wait>0` 才真排队。

## 3. 校验不变量（跑完必看）

```bash
curl -s http://127.0.0.1:8099/stats
# {"acquired":..,"busy":..,"errors":0,"released":..,"activeHolds":0,
#  "mutexViolations":0,"fenceMonotonicBreaks":0}
```

`mutexViolations` 或 `fenceMonotonicBreaks` > 0 → **锁不安全，FAIL**。`activeHolds` 稳态应配平回落，
**收尾可能残留 1~2**（JMeter 在 duration 边界把某个 acquire↔release 配对切断了；对应 Redis 锁靠 lease
自动过期，不影响互斥判定）。多轮之间用 `POST /reset`（清零计数 + 主动释放残留句柄）或重启 loadserver 清零。

## 4. Grafana（可选）

loadserver 的 `/metrics` 出 `webook_lock_*`（acquire_total / held_seconds / wait_seconds /
watchdog_lost_total / fence_issued_total）。prometheus 抓 `127.0.0.1:8099/metrics` 即可复用现有看板。

## JMeter 属性（`-J`）

| 属性 | 默认 | 含义 |
|------|------|------|
| `host` / `port` | `127.0.0.1` / `8099` | loadserver 地址 |
| `threads` | `64` | 并发线程（= 并发抢锁数） |
| `rampup` | `5` | 线程爬升秒数 |
| `duration` | `60` | 压测时长秒 |
| `key` | `jmeter:lock` | 竞争 key（同一个 = 最强竞争；用 `${__threadNum}` 拼可分散） |
| `hold` | `20` | 持锁 think-time 毫秒（模拟临界区，acquire 与 release 之间） |
| `lease` | `0s` | 租约；`0s`=watchdog 自动续约（hold 再长也不过期），`>0`=固定租约 |
| `watchdog` | `30s` | `lease=0s` 时的 watchdog 超时（续约每 /3）。调小（如 `500ms`）+ 长 `hold` 才压得到续约/丢锁路径 |
| `wait` | `0s` | TryLock 等待上限；`0s`=拿不到立即 busy |
| `fencing` | `true` | 是否启用 fencing（校验令牌单调） |
| `fair` | `false` | 公平锁 FIFO 排队；**需配 `wait`>0** 才排队等待（否则拿不到即 busy），且须 `fencing=false`（不与 fencing 组合） |

## 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/acquire?key=&lease=&wait=&fencing=&fair=` | 200 `{token,fence}` / 409 `{busy:true}` / 500 |
| POST | `/release?key=&token=` | 200 `{released:true}` / 410（未知/已释放 token） |
| GET | `/stats` | 计数 + 不变量违规数（JSON） |
| POST | `/reset` | 清零计数 + 释放残留句柄（多轮压测之间调，免重启） |
| GET | `/metrics` | prometheus `webook_lock_*` |
| GET | `/healthz` | `ok` |

## 注意

- **`lease>0` 且 `hold > lease` 且无 watchdog** → 锁会在持有期内过期被他人抢走，`mutexViolations` 会
  真的 >0。这是**设计使然的正确暴露**（租约配短了）：要么调大 `lease`，要么 `lease=0s` 用 watchdog。
- loadserver 把句柄按 token 存在内存 map，跨 `/acquire`→`/release` 两次请求持有；JMeter 计划保证
  每个 200 的 acquire 都配一次 release（`if acquired` 控制器），不泄漏。