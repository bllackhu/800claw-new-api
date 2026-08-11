-- Fixed-window atomic multi-policy enforcement
-- KEYS[1..N]: counter keys for each policy
-- ARGV[1..N]: limit counts (same index as KEY)
-- ARGV[N+1..2N]: window_seconds for TTL
-- ARGV[2N+1..3N]: increment delta (float) for each counter
local incremented = {}
for i = 1, #KEYS do
    local delta = tonumber(ARGV[2 * #KEYS + i])
    -- INCRBYFLOAT replies with a bulk string (e.g. "3.5"); convert to a number
    -- so the comparison below does not raise "attempt to compare string with number".
    local count = tonumber(redis.call('INCRBYFLOAT', KEYS[i], delta))
    redis.call('EXPIRE', KEYS[i], tonumber(ARGV[#KEYS + i]) + 60)
    table.insert(incremented, {key = KEYS[i], delta = delta})
    if count > tonumber(ARGV[i]) then
        for _, entry in ipairs(incremented) do
            redis.call('INCRBYFLOAT', entry.key, -entry.delta)
        end
        return 0
    end
end
return 1
