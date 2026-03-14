-- Remove seeded Core Pack agents
DELETE FROM agent_definitions WHERE name IN (
    'log-analyzer',
    'security-scanner',
    'system-monitor',
    'deployment-assistant',
    'agent-builder'
) AND is_builtin = TRUE;
