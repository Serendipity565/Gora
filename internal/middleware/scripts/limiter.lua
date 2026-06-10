-- KEYS[1] = availableTokens key
-- KEYS[2] = latestTick key

-- ARGV[1] = capacity
-- ARGV[2] = quantum
-- ARGV[3] = fillInterval
-- ARGV[4] = now (milliseconds)
-- ARGV[5] = count (取多少 token)

local availableKey = KEYS[1]
local latestKey    = KEYS[2]

local capacity     = tonumber(ARGV[1])
local quantum      = tonumber(ARGV[2])
local fillInterval = tonumber(ARGV[3])
local now          = tonumber(ARGV[4])
local count        = tonumber(ARGV[5])

if capacity == nil or quantum == nil or fillInterval == nil or now == nil or count == nil then
    return redis.error_reply("invalid args")
end

-- ==== Step 1: 获取 Redis 中的状态 ====
local available = tonumber(redis.call("GET", availableKey))
local latestTime = tonumber(redis.call("GET", latestKey))

if not available then
    available = capacity
end
if not latestTime then
    latestTime = now
end

if latestTime > now then
    latestTime = now
end

-- ==== Step 2: 补充 token ====
local interval_ms
if fillInterval <= 0 then
    interval_ms = 1000  -- 兜底：每秒 1 次
else
    interval_ms = 1000.0 / fillInterval
end

local elapsed = now - latestTime
if elapsed >= interval_ms then
    local fills = math.floor(elapsed / interval_ms)
    if fills > 0 then
        local newTokens = fills * quantum
        available = math.min(capacity, available + newTokens)
        latestTime = latestTime + fills * interval_ms
    end
end

-- ==== Step 3: 尝试取 token ====
if available >= count then
    available = available - count
    redis.call("SET", availableKey, available)
    redis.call("EXPIRE", availableKey, 3600)  -- 1 hour TTL
    redis.call("SET", latestKey, math.floor(latestTime))
    redis.call("EXPIRE", latestKey, 3600)
    return 1   -- success
else
    redis.call("SET", availableKey, available)
    redis.call("EXPIRE", availableKey, 3600)  -- 1 hour TTL
    redis.call("SET", latestKey, math.floor(latestTime))
    redis.call("EXPIRE", latestKey, 3600)
    return 0   -- not enough tokens
end
