-- +goose Up
DELETE FROM
  feed_events
WHERE
  actor_user_id IS NOT NULL
  AND actor_user_id = user_id;

-- +goose Down
SELECT
  1;
