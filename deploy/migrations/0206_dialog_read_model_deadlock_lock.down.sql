-- Revert 0205: restore rugram_bump_dialog_light to the prep-advisory-lock
-- body (identical to migrations/0195/0196 state).

CREATE OR REPLACE FUNCTION public.rugram_bump_dialog_light(
    p_owner_user_id bigint,
    p_peer_type text,
    p_peer_id bigint
)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    IF COALESCE(p_owner_user_id, 0) = 0
       OR COALESCE(p_peer_id, 0) = 0
       OR COALESCE(p_peer_type, '') = ''
    THEN
        RETURN;
    END IF;

    PERFORM public.rugram_bump_read_model_version(
        'dialog_light', p_owner_user_id, p_peer_type, p_peer_id
    );
    PERFORM public.rugram_bump_dialog_owner(p_owner_user_id);
END;
$$;