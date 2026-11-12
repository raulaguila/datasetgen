DO $$
BEGIN
  IF to_regclass('cron.job') IS NOT NULL THEN
    PERFORM cron.unschedule('dataset_cleanup');
    PERFORM cron.unschedule('dataset_rate_limit');
  END IF;
END;
$$;

DROP FUNCTION sys.fn_cleanup(INTERVAL);
DROP TABLE sys.rate_limit, sys.job, sys.outbox;
