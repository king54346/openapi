1. 登录会话纳入 Redis + 服务端强制踢下线
启用 Redis 时，浏览器 Cookie 仅存放签名后的 session_id，会话负载存入 openapi:session:<sid>，并维护用户级会话集合 openapi:user_sessions:<uid>；未配置 Redis 时自动回退至 Cookie Store

2.  本地 L1 内存缓存 + Redis Pub/Sub 毫秒级跨节点同步
Token / User 缓存（auth_cache.go）：广播 token:invalidate / user:invalidate，多节点毫秒级剔除失效凭证与被封禁用户
    Token 变更/禁用/删除 先删本机 tokenCache，再通过 PublishClusterEvent 向 Redis 频道 openapi:cluster:events 广播 token:invalidate
    用户禁用/改角色/改信息/删除 先删本机 userCache，再广播 user:invalidate

渠道增删改或自动封禁时广播 channel:refresh 全集群即时重载；启用 Redis 时兜底定时同步自动放宽至 >= 300s，且定时同步未发生数量变化时不再重复刷屏日志
系统配置 OptionMap & 模型标签缓存（option.go、model_meta.go）：广播 option:refresh 与 model_tags:invalidate。

3. 跨节点会话亲和性、集群熔断计数与多密钥原子轮询
跨节点 Session Affinity（affinity.go）：启用 Redis 时将亲和映射（openapi:affinity:entry:...）、命中统计与各渠道命中计数统一写入 Redis，多实例负载均衡下同一对话稳定命中同一上游渠道（支持前缀缓存复用），异常或未启用时无缝降级本地内存。
集群级连续失败自动封禁（health.go）：通过 openapi:health:fail:<channelId> 原子累加全集群失败次数，成功时清零，避免多节点流量均摊导致故障渠道迟迟无法触发熔断。
多 Key 渠道零写库轮询（channel.go）：MultiKeyModePolling 模式下改用 Redis INCR openapi:channel:polling:<channelId> 原子游标取模选 Key，彻底消除高并发下每次请求写数据库的开销。

4. 对等节点后台任务分布式抢锁
数据库启动迁移（main.go）：多实例并发冷启动时通过 db:migrate 锁串行化执行 AutoMigrate。
Cron 定时调度器（scheduler.go）：所有节点常驻调度器，触发时先抢分钟级幂等锁 cron:tick:<jobId>:<minute>（防时钟微偏重复触发），再持自动续租锁 cron:run:<jobId> 执行；任一节点停机后其余节点下一周期自动接管。
异步视频/任务轮询器（poller.go）：默认开启轮询，每轮通过 task:poll_cycle 分布式锁确保同一周期仅单节点拉取上游任务状态。