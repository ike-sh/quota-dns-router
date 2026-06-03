ALTER TABLE dns_switch_history
    ADD COLUMN trigger_type TEXT NOT NULL DEFAULT 'threshold';

UPDATE dns_switch_history
SET trigger_type = CASE
    WHEN lower(trigger_type) != 'threshold' THEN trigger_type
    WHEN reason = '手动切换' THEN 'manual'
    WHEN reason = '当前节点离线' THEN 'offline'
    WHEN reason = '当前节点已禁用' THEN 'disabled'
    WHEN reason = '当前节点不参与自动切换' THEN 'disabled'
    ELSE 'threshold'
END;
