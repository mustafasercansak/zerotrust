CREATE OR REPLACE FUNCTION notify_sa_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_notify('service_accounts_changed', TG_OP);
  RETURN NULL;
END;
$$;

CREATE TRIGGER sa_change_trigger
AFTER INSERT OR UPDATE OR DELETE ON service_accounts
FOR EACH STATEMENT EXECUTE FUNCTION notify_sa_change();
