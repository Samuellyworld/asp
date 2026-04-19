-- allow scanner decisions filtered by confidence decay near a user's daily limit.

ALTER TABLE ai_decisions
    DROP CONSTRAINT IF EXISTS ai_decisions_filter_reason_check;

ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_filter_reason_check
    CHECK (filter_reason IN (
        'none',
        'hold',
        'low_confidence',
        'confidence_decay',
        'duplicate',
        'daily_limit',
        'safety_blocked',
        'expired',
        'user_rejected'
    ));
