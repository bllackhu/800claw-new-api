-- Fixed-window atomic multi-policy enforcement
-- KEYS[1..N]: counter keys for each policy
-- ARGV[1..N]: limit counts (same index as KEY)
-- ARGV[N+1..2N]: window_seconds for TTL
local incremented = {}
for i = 1, #KEYS do
    local count = redis.call('INCR', KEYS[i])
    redis.call('EXPIRE', KEYS[i], tonumber(ARGV[#KEYS + i]) + 60)
    table.insert(incremented, KEYS[i])
    if count > tonumber(ARGV[i]) then
        for _, key in ipairs(incremented) do
            redis.call('DECR', key)
        end
        return 0
    end
end
return 1