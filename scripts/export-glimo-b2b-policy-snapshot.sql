WITH selected_options AS (
  SELECT jsonb_object_agg(key, value::jsonb) AS value
  FROM options
  WHERE key IN (
    'AutoGroups',
    'CacheRatio',
    'CompletionRatio',
    'CreateCacheRatio',
    'DefaultUseAutoGroup',
    'GroupRatio',
    'ImageRatio',
    'ModelRatio',
    'TopupGroupRatio',
    'UserUsableGroups',
    'group_ratio_setting.group_ratio',
    'group_ratio_setting.group_special_usable_group'
  )
), selected_abilities AS (
  SELECT COALESCE(
    jsonb_agg(
      jsonb_build_object(
        'group', a."group",
        'model', a.model,
        'channelId', a.channel_id,
        'abilityEnabled', a.enabled,
        'channelStatus', c.status
      ) ORDER BY a."group", a.model, a.channel_id
    ),
    '[]'::jsonb
  ) AS value
  FROM abilities a
  JOIN channels c ON c.id = a.channel_id
  WHERE a."group" IN ('b2b', 'b2b-deepseek')
), b2b_users AS (
  SELECT count(*)::integer AS value
  FROM users
  WHERE deleted_at IS NULL AND "group" = 'b2b'
), b2b_tokens AS (
  SELECT
    count(*)::integer AS total,
    count(*) FILTER (
      WHERE t."group" IS DISTINCT FROM 'auto'
         OR t.cross_group_retry IS DISTINCT FROM false
    )::integer AS violations
  FROM tokens t
  JOIN users u ON u.id = t.user_id
  WHERE t.deleted_at IS NULL
    AND u.deleted_at IS NULL
    AND u."group" = 'b2b'
)
SELECT jsonb_build_object(
  'capturedAt', to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
  'options', COALESCE((SELECT value FROM selected_options), '{}'::jsonb),
  'abilities', (SELECT value FROM selected_abilities),
  'b2bUserCount', (SELECT value FROM b2b_users),
  'b2bTokenCount', (SELECT total FROM b2b_tokens),
  'b2bTokenViolationCount', (SELECT violations FROM b2b_tokens)
)::text;
