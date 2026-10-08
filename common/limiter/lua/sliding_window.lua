-- 滑动窗口限流（Redis list 存请求时间戳，新 -> 旧）
-- KEYS[1]: 限流 key
-- ARGV[1]: 窗口内最大请求数
-- ARGV[2]: 窗口时长（毫秒）
-- ARGV[3]: key 过期时间（毫秒）
-- ARGV[4]: 模式 0=只检查 1=检查并记录 2=只记录
-- 返回 1 放行，0 拒绝

local key = KEYS[1]
local max = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local mode = tonumber(ARGV[4])

-- 使用 Redis 服务器时间，避免多实例时钟不一致
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)

if mode ~= 2 and redis.call('LLEN', key) >= max then
    -- 无法解析的旧数据视为已过期
    local oldest = tonumber(redis.call('LINDEX', key, -1))
    if oldest and now - oldest < window then
        redis.call('PEXPIRE', key, ttl)
        return 0
    end
end

if mode ~= 0 then
    redis.call('LPUSH', key, now)
    redis.call('LTRIM', key, 0, max - 1)
    redis.call('PEXPIRE', key, ttl)
end
return 1
