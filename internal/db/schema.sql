PRAGMA foreign_keys = ON;

CREATE TABLE application_settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE policy_settings (
    scope TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL,
    profile TEXT NOT NULL,
    rules_json TEXT NOT NULL DEFAULT '[]',
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (scope, project_id)
);

CREATE TABLE skill_registrations (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    path TEXT,
    name TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE mcp_servers (
    name TEXT NOT NULL,
    scope TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (name, scope, project_id)
);

CREATE TABLE configured_providers (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    name TEXT NOT NULL,
    remark TEXT,
    protocol TEXT NOT NULL,
    api_endpoint TEXT NOT NULL,
    api_key TEXT,
    api_key_secret_ref TEXT,
    proxy TEXT,
    default_model TEXT,
    models_json TEXT NOT NULL DEFAULT '[]',
    default_context_window INTEGER,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (provider_id) REFERENCES provider_catalog(id)
);

CREATE TABLE model_metadata_cache (
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    context_window INTEGER,
    max_output_tokens INTEGER,
    source TEXT NOT NULL,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (provider_id, model_id)
);

CREATE TABLE files (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    path TEXT NOT NULL,
    content TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,  -- Unix timestamp in milliseconds
    updated_at INTEGER NOT NULL,  -- Unix timestamp in milliseconds
    FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE,
    UNIQUE(path, session_id, version)
);

CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    parts TEXT NOT NULL default '[]',
    model TEXT,
    created_at INTEGER NOT NULL,  -- Unix timestamp in milliseconds
    updated_at INTEGER NOT NULL,  -- Unix timestamp in milliseconds
    finished_at INTEGER, provider TEXT, is_summary_message INTEGER DEFAULT 0 NOT NULL, metadata_json TEXT, usage_json TEXT,  -- Unix timestamp in milliseconds
    FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE
);

CREATE TABLE project_memory_injections (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    prompt_assembly_id TEXT,
    injected_at TEXT NOT NULL,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    selection_reason TEXT NOT NULL DEFAULT '',
    FOREIGN KEY(memory_id) REFERENCES project_memory_records(id)
);

CREATE TABLE project_memory_records (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    relative_path TEXT NOT NULL,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    tags_json TEXT NOT NULL DEFAULT '[]',
    content_hash TEXT NOT NULL,
    mtime_unix INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_from_session_id TEXT,
    created_from_turn_id TEXT,
    last_indexed_at TEXT NOT NULL,
    last_injected_at TEXT,
    UNIQUE(project_id, relative_path)
);

CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    canonical_path TEXT NOT NULL,
    git_root TEXT,
    branch TEXT,
    is_git_repository INTEGER NOT NULL DEFAULT 0,
    exists_on_disk INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_opened_at INTEGER,
    deleted_at INTEGER
);

CREATE TABLE provider_catalog (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    api_endpoint TEXT,
    api_key_template TEXT,
    model_count INTEGER NOT NULL DEFAULT 0,
    default_large_model TEXT,
    default_small_model TEXT,
    required_fields_json TEXT NOT NULL DEFAULT '[]',
    notes_json TEXT NOT NULL DEFAULT '[]',
    configurable INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE read_files (
    session_id TEXT NOT NULL CHECK (session_id != ''),
    path TEXT NOT NULL CHECK (path != ''),
    read_at INTEGER NOT NULL, turn_id TEXT NOT NULL DEFAULT '', tool_call_id TEXT NOT NULL DEFAULT '', size_bytes INTEGER NOT NULL DEFAULT 0, content_hash TEXT NOT NULL DEFAULT '', mtime_unix INTEGER NOT NULL DEFAULT 0, offset INTEGER NOT NULL DEFAULT 0, read_limit INTEGER NOT NULL DEFAULT 0, partial INTEGER NOT NULL DEFAULT 0, token_estimate INTEGER NOT NULL DEFAULT 0, state TEXT NOT NULL DEFAULT 'recorded', reason TEXT NOT NULL DEFAULT '',  -- Unix timestamp in seconds when file was last read
    FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE,
    PRIMARY KEY (path, session_id)
);

CREATE TABLE runtime_agent_roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    title TEXT,
    description TEXT,
    prompt_summary TEXT,
    allowed_tools_json TEXT,
    capability_scope_json TEXT,
    model TEXT,
    provider TEXT,
    cwd TEXT,
    worktree TEXT,
    risk TEXT,
    policy_metadata_json TEXT,
    source TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE runtime_agent_task_messages (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    parent_task_id TEXT,
    parent_turn_id TEXT,
    parent_session_id TEXT,
    child_session_id TEXT,
    direction TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    content_summary TEXT,
    payload_json TEXT,
    related_tool_call_id TEXT,
    related_message_id TEXT,
    artifact_refs_json TEXT,
    created_at INTEGER NOT NULL,
    delivered_at INTEGER
, sequence INTEGER NOT NULL DEFAULT 0, processed_at INTEGER, error TEXT);

CREATE TABLE runtime_agent_task_results (
    task_id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    summary TEXT,
    error_detail TEXT,
    cancellation_detail TEXT,
    artifact_refs_json TEXT,
    related_message_refs_json TEXT,
    related_tool_call_refs_json TEXT,
    compact_boundary_refs_json TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE runtime_agent_tasks (
    id TEXT PRIMARY KEY,
    parent_turn_id TEXT,
    parent_session_id TEXT NOT NULL,
    parent_tool_call_id TEXT,
    parent_task_id TEXT,
    child_session_id TEXT,
    team_id TEXT,
    dependencies_json TEXT,
    title TEXT NOT NULL,
    kind TEXT NOT NULL,
    role TEXT,
    name TEXT,
    prompt_summary TEXT,
    model TEXT,
    provider TEXT,
    allowed_tools_json TEXT,
    capability_scope_json TEXT,
    cwd TEXT,
    worktree TEXT,
    status TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    result_summary TEXT,
    artifact_refs_json TEXT,
    started_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    finished_at INTEGER,
    error TEXT
);

CREATE TABLE runtime_audit_events (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    turn_id TEXT,
    type TEXT NOT NULL,
    created_at TEXT NOT NULL,
    payload_json TEXT NOT NULL
, tool_call_id TEXT, permission_id TEXT);

CREATE TABLE runtime_context_boundaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    kind TEXT NOT NULL,
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    summary_message_id TEXT,
    summary_ref TEXT,
    message_refs_json TEXT,
    preserved_message_refs_json TEXT,
    boundary_cutoff_message_id TEXT,
    summary_mode TEXT,
    memory_revision INTEGER,
    tool_call_refs_json TEXT,
    reinjected_refs_json TEXT,
    budget_before_json TEXT,
    budget_after_json TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    error TEXT
);

CREATE TABLE runtime_session_memory_revisions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    revision INTEGER NOT NULL,
    status TEXT NOT NULL,
    base_revision INTEGER,
    content TEXT,
    content_hash TEXT,
    last_summarized_message_id TEXT,
    source_message_count INTEGER NOT NULL DEFAULT 0,
    source_token_estimate INTEGER NOT NULL DEFAULT 0,
    source_tool_call_count INTEGER NOT NULL DEFAULT 0,
    provider TEXT,
    model TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    error TEXT,
    UNIQUE (session_id, revision)
);

CREATE TABLE runtime_context_content_replacements (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    tool_call_id TEXT,
    tool_name TEXT,
    kind TEXT NOT NULL,
    original_ref TEXT,
    replacement_text TEXT NOT NULL,
    original_size_bytes INTEGER NOT NULL DEFAULT 0,
    original_estimated_tokens INTEGER NOT NULL DEFAULT 0,
    replacement_estimated_tokens INTEGER NOT NULL DEFAULT 0,
    reason TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE runtime_context_projection_messages (
    id TEXT PRIMARY KEY,
    projection_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    role TEXT NOT NULL,
    canonical_message_id TEXT,
    status TEXT NOT NULL,
    replacement_id TEXT,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    content_ref TEXT,
    summary TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE runtime_context_projections (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    step INTEGER NOT NULL,
    provider TEXT,
    model TEXT,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    canonical_message_count INTEGER NOT NULL DEFAULT 0,
    projected_message_count INTEGER NOT NULL DEFAULT 0,
    budget_before_json TEXT,
    budget_after_json TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    error TEXT
);

CREATE TABLE runtime_context_reactive_attempts (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    projection_id TEXT,
    attempt INTEGER NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL,
    error TEXT,
    budget_before_json TEXT,
    budget_after_json TEXT,
    will_retry INTEGER NOT NULL DEFAULT 0,
    circuit_open INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE TABLE runtime_context_circuit_states (
    session_id TEXT PRIMARY KEY,
    failure_count INTEGER NOT NULL DEFAULT 0,
    circuit_open INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

CREATE TABLE runtime_context_read_state_snapshots (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    state_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE runtime_context_reinjections (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    kind TEXT NOT NULL,
    ref TEXT,
    status TEXT NOT NULL,
    reason TEXT,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE runtime_context_snip_boundaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    removed_message_refs_json TEXT,
    preserved_head_ref TEXT,
    preserved_tail_ref TEXT,
    summary_ref TEXT,
    reason TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE runtime_context_warnings (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    code TEXT NOT NULL,
    message TEXT NOT NULL,
    severity TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE runtime_events (
    sequence INTEGER PRIMARY KEY,
    id TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    session_id TEXT,
    turn_id TEXT,
    message_id TEXT,
    tool_call_id TEXT,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE conversation_entity_events_v2 (
    session_id TEXT NOT NULL,
    raw_sequence INTEGER NOT NULL,
    ordinal INTEGER NOT NULL,
    event_id TEXT NOT NULL UNIQUE,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    revision TEXT NOT NULL,
    event_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, raw_sequence, ordinal),
    UNIQUE (session_id, raw_sequence, entity_type, entity_id, operation)
);

CREATE TABLE conversation_projector_checkpoints_v2 (
    session_id TEXT PRIMARY KEY,
    last_raw_sequence INTEGER NOT NULL,
    failure_reason TEXT,
    updated_at INTEGER NOT NULL
);

CREATE TABLE conversation_projector_batches_v2 (
    session_id TEXT NOT NULL,
    raw_sequence INTEGER NOT NULL,
    previous_raw_sequence INTEGER NOT NULL,
    entity_count INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, raw_sequence)
);

CREATE TABLE conversation_entities_v2 (
    session_id TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    turn_id TEXT,
    activity_sequence TEXT NOT NULL,
    revision TEXT NOT NULL,
    entity_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, entity_type, entity_id)
);

CREATE TABLE runtime_hook_executions (
    id TEXT PRIMARY KEY,
    hook_id TEXT NOT NULL,
    hook_name TEXT,
    hook_source TEXT,
    event TEXT NOT NULL,
    status TEXT NOT NULL,
    session_id TEXT,
    turn_id TEXT,
    tool_call_id TEXT,
    task_id TEXT,
    capability_id TEXT,
    mcp_server TEXT,
    skill TEXT,
    context_ref TEXT,
    policy_mode TEXT,
    policy_profile TEXT,
    policy_rule TEXT,
    policy_decision TEXT,
    policy_reason TEXT,
    headless INTEGER NOT NULL DEFAULT 0,
    headless_reason TEXT,
    sandbox_decision_id TEXT,
    sandbox_status TEXT,
    scope_kind TEXT,
    scope_value TEXT,
    reason TEXT,
    error TEXT,
    input_summary TEXT,
    output_summary TEXT,
    context_summary TEXT,
    input_rewritten INTEGER NOT NULL DEFAULT 0,
    context_injected INTEGER NOT NULL DEFAULT 0,
    redacted INTEGER NOT NULL DEFAULT 1,
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    duration_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE runtime_mcp_requests (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    server TEXT NOT NULL,
    capability_id TEXT,
    session_id TEXT,
    turn_id TEXT,
    status TEXT NOT NULL,
    prompt TEXT,
    description TEXT,
    response_summary TEXT,
    policy_mode TEXT,
    policy_profile TEXT,
    policy_decision TEXT,
    policy_reason TEXT,
    policy_risk TEXT,
    policy_rule_id TEXT,
    policy_rule_source TEXT,
    policy_scope_kind TEXT,
    policy_scope_value TEXT,
    policy_target_summary TEXT,
    policy_headless INTEGER DEFAULT 0,
    policy_headless_reason TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    expires_at INTEGER,
    completed_at INTEGER,
    error TEXT,
    redacted INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE runtime_permission_requests (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    tool_call_id TEXT,
    tool_name TEXT NOT NULL,
    description TEXT,
    action TEXT NOT NULL,
    params_json TEXT,
    path TEXT,
    target TEXT,
    risk TEXT,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    decided_at INTEGER
, policy_mode TEXT, policy_reason TEXT, decision TEXT, policy_profile TEXT, policy_rule_id TEXT, policy_rule_source TEXT, policy_scope_kind TEXT, policy_scope_value TEXT, policy_target_summary TEXT, policy_headless INTEGER DEFAULT 0, policy_headless_reason TEXT);

CREATE TABLE runtime_prompt_assemblies (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    step INTEGER NOT NULL,
    provider TEXT,
    model TEXT,
    system_json TEXT,
    messages_json TEXT,
    tools_json TEXT,
    skills_json TEXT,
    mcp_json TEXT,
    context_sources_json TEXT,
    compact_json TEXT,
    budget_json TEXT,
    created_at INTEGER NOT NULL
, projection_id TEXT, sections_json TEXT);

CREATE TABLE runtime_recovery_links (
  id TEXT PRIMARY KEY,
  source_turn_id TEXT NOT NULL,
  resumed_turn_id TEXT NOT NULL,
  action TEXT NOT NULL,
  mode TEXT NOT NULL,
  interruption_kind TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE objects (
    id TEXT PRIMARY KEY,
    uri TEXT NOT NULL UNIQUE,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    tool_call_id TEXT,
    task_id TEXT,
    kind TEXT NOT NULL,
    media_type TEXT,
    content_type TEXT,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    estimated_tokens INTEGER NOT NULL DEFAULT 0,
    preview TEXT,
    summary TEXT,
    storage_kind TEXT NOT NULL,
    storage_path TEXT,
    inline_payload TEXT,
    redaction_status TEXT NOT NULL,
    created_at INTEGER NOT NULL
, sandbox_decision_id TEXT, sandbox_mode TEXT, sandbox_status TEXT);

CREATE TABLE runtime_run_checkpoints (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    task_id TEXT,
    status TEXT NOT NULL,
    summary TEXT,
    artifact_refs_json TEXT,
    diagnostic_refs_json TEXT,
    created_at INTEGER NOT NULL,
    acknowledged_at INTEGER,
    discarded_at INTEGER,
    metadata_json TEXT
);

CREATE TABLE runtime_run_sessions (
    run_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    task_id TEXT,
    turn_id TEXT,
    worktree_id TEXT,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (run_id, session_id)
);

CREATE TABLE runtime_run_transitions (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    session_id TEXT,
    turn_id TEXT,
    task_id TEXT,
    from_status TEXT,
    to_status TEXT NOT NULL,
    reason TEXT,
    source TEXT NOT NULL,
    event_id TEXT,
    created_at INTEGER NOT NULL,
    metadata_json TEXT
);

CREATE TABLE runtime_runs (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    primary_session_id TEXT NOT NULL,
    objective TEXT,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    finished_at INTEGER,
    discarded_at INTEGER,
    source TEXT NOT NULL,
    metadata_json TEXT
);

CREATE TABLE runtime_sandbox_decisions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    tool_call_id TEXT,
    task_id TEXT,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    executor TEXT,
    cwd TEXT,
    worktree_id TEXT,
    worktree_path TEXT,
    command_summary TEXT,
    policy_mode TEXT,
    policy_profile TEXT,
    policy_rule TEXT,
    reason TEXT,
    error TEXT,
    allowed_paths_json TEXT,
    denied_paths_json TEXT,
    network_allowed INTEGER NOT NULL DEFAULT 0,
    network_reason TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE TABLE runtime_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    checksum TEXT NOT NULL,
    app_version TEXT NOT NULL,
    applied_at INTEGER NOT NULL
);

INSERT INTO schema_migrations(version,name,kind,checksum,app_version,applied_at)
VALUES(1,'v0.1.0-baseline','baseline','embedded-schema','development',strftime('%s','now') * 1000);
INSERT INTO schema_migrations(version,name,kind,checksum,app_version,applied_at)
VALUES(2,'token-statistics','migration','20260715000000','development',strftime('%s','now') * 1000);

CREATE TABLE runtime_token_usage_daily (
    day TEXT PRIMARY KEY,
    timezone TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    session_count INTEGER NOT NULL DEFAULT 0,
    turn_count INTEGER NOT NULL DEFAULT 0,
    model_call_count INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

CREATE TABLE runtime_token_usage_lifetime (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    model_call_count INTEGER NOT NULL DEFAULT 0,
    peak_tokens INTEGER NOT NULL DEFAULT 0,
    peak_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

CREATE TABLE token_statistics_cursor (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    sequence INTEGER NOT NULL DEFAULT 0,
    backfilled INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

INSERT INTO runtime_token_usage_lifetime (id, updated_at) VALUES (1, strftime('%s','now') * 1000);
INSERT INTO token_statistics_cursor (id, updated_at) VALUES (1, strftime('%s','now') * 1000);

CREATE TABLE runtime_tool_calls (
    id TEXT PRIMARY KEY,
    turn_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    message_id TEXT,
    name TEXT NOT NULL,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    input_summary TEXT,
    output_summary TEXT,
    stdout TEXT,
    stderr TEXT,
    is_error INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL,
    finished_at INTEGER,
    error TEXT
, model_content TEXT, structured_output TEXT, capability_id TEXT NOT NULL DEFAULT '', job_id TEXT, command TEXT, risk TEXT, policy_reason TEXT, exit_code INTEGER NOT NULL DEFAULT 0, job_status TEXT, job_started_at INTEGER, job_finished_at INTEGER, compacted INTEGER NOT NULL DEFAULT 0, compact_ref TEXT, compact_boundary_id TEXT, compact_original_estimated_tokens INTEGER NOT NULL DEFAULT 0, compacted_at INTEGER, policy_mode TEXT, policy_profile TEXT, policy_rule_id TEXT, policy_rule_source TEXT, policy_scope_kind TEXT, policy_scope_value TEXT, policy_target_summary TEXT, shell_risk TEXT, shell_reason TEXT, policy_headless INTEGER DEFAULT 0, policy_headless_reason TEXT, output_refs_json TEXT, artifact_refs_json TEXT, diff_refs_json TEXT, sandbox_decision_id TEXT, sandbox_mode TEXT, sandbox_status TEXT, sandbox_executor TEXT, sandbox_reason TEXT, sandbox_error TEXT);

CREATE TABLE runtime_turns (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL,
    user_message_id TEXT,
    latest_assistant_message_id TEXT,
    provider TEXT,
    model TEXT,
    prompt_preview TEXT,
    usage_before_json TEXT,
    usage_after_json TEXT,
    usage_delta_json TEXT,
    started_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    finished_at INTEGER,
    error TEXT
);

CREATE TABLE runtime_user_inputs (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    turn_id TEXT,
    project_id TEXT,
    scope TEXT,
    mode TEXT NOT NULL,
    prompt_preview TEXT,
    items_json TEXT NOT NULL,
    normalized_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE runtime_worktrees (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    task_id TEXT,
    base_repo_path TEXT NOT NULL,
    worktree_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    ref TEXT,
    status TEXT NOT NULL,
    preserve_policy TEXT NOT NULL,
    cleanup_policy TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    entered_at INTEGER,
    exited_at INTEGER,
    cleaned_at INTEGER,
    updated_at INTEGER NOT NULL,
    error TEXT,
    owner TEXT,
    metadata_json TEXT
);

CREATE TABLE selected_models (
    id TEXT PRIMARY KEY,
    configured_provider_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'global',
    project_id TEXT,
    session_id TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (configured_provider_id) REFERENCES configured_providers(id)
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    parent_session_id TEXT,
    title TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('project', 'standalone')),
    project_id TEXT REFERENCES projects(id) ON DELETE RESTRICT,
    workdir TEXT,
    canonical_workdir TEXT,
    workdir_exists INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'active',
    title_source TEXT NOT NULL DEFAULT 'auto',
    pinned INTEGER NOT NULL DEFAULT 0,
    message_count INTEGER NOT NULL DEFAULT 0 CHECK (message_count >= 0),
    prompt_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (completion_tokens>= 0),
    cost REAL NOT NULL DEFAULT 0.0 CHECK (cost >= 0.0),
    updated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    last_opened_at INTEGER,
    deleted_at INTEGER,
    summary_message_id TEXT,
    todos TEXT,
    CHECK (
        (
            scope = 'standalone' AND
            project_id IS NULL AND
            workdir IS NOT NULL AND
            length(trim(workdir)) > 0
        ) OR
        (scope = 'project' AND project_id IS NOT NULL)
    )
);

CREATE INDEX idx_configured_providers_enabled
    ON configured_providers (enabled);

CREATE INDEX idx_configured_providers_provider_id
    ON configured_providers (provider_id);

CREATE INDEX idx_files_created_at ON files (created_at);

CREATE INDEX idx_files_path ON files (path);

CREATE INDEX idx_files_session_id ON files (session_id);

CREATE INDEX idx_messages_created_at ON messages (created_at);

CREATE INDEX idx_messages_session_id ON messages (session_id);

CREATE INDEX idx_project_memory_injections_turn
    ON project_memory_injections(session_id, turn_id);

CREATE INDEX idx_project_memory_records_project
    ON project_memory_records(project_id, enabled, deleted_at, updated_at);

CREATE INDEX idx_project_memory_records_type
    ON project_memory_records(project_id, type);

CREATE UNIQUE INDEX idx_projects_active_canonical_path
ON projects(canonical_path)
WHERE deleted_at IS NULL;

CREATE INDEX idx_projects_last_opened
ON projects(deleted_at, last_opened_at DESC);

CREATE INDEX idx_projects_active_recent
ON projects(deleted_at, last_opened_at DESC, updated_at DESC);

CREATE INDEX idx_read_files_path ON read_files (path);

CREATE INDEX idx_read_files_session_id ON read_files (session_id);

CREATE INDEX idx_read_files_session_turn ON read_files (session_id, turn_id);

CREATE INDEX idx_runtime_agent_task_messages_parent_turn_created_at
    ON runtime_agent_task_messages (parent_turn_id, created_at);

CREATE INDEX idx_runtime_agent_task_messages_task_created_at
    ON runtime_agent_task_messages (task_id, created_at);

CREATE INDEX idx_runtime_agent_task_messages_task_sequence
    ON runtime_agent_task_messages (task_id, sequence);

CREATE INDEX idx_runtime_agent_tasks_child_session_id
    ON runtime_agent_tasks (child_session_id);

CREATE INDEX idx_runtime_agent_tasks_parent_session_updated_at
    ON runtime_agent_tasks (parent_session_id, updated_at);

CREATE INDEX idx_runtime_agent_tasks_parent_turn_updated_at
    ON runtime_agent_tasks (parent_turn_id, updated_at);

CREATE INDEX idx_runtime_agent_tasks_status_updated_at
    ON runtime_agent_tasks (status, updated_at);

CREATE INDEX idx_runtime_audit_events_permission_id_created_at
    ON runtime_audit_events (permission_id, created_at);

CREATE INDEX idx_runtime_audit_events_tool_call_id_created_at
    ON runtime_audit_events (tool_call_id, created_at);

CREATE INDEX idx_runtime_audit_events_turn_id_created_at
    ON runtime_audit_events (turn_id, created_at);

CREATE INDEX idx_runtime_context_boundaries_projection_created_at
    ON runtime_context_boundaries (projection_id, created_at);

CREATE INDEX idx_runtime_context_boundaries_turn_created_at
    ON runtime_context_boundaries (turn_id, created_at);

CREATE INDEX idx_runtime_session_memory_status_revision
    ON runtime_session_memory_revisions (session_id, status, revision);

CREATE INDEX idx_runtime_context_content_replacements_projection
    ON runtime_context_content_replacements (projection_id);

CREATE INDEX idx_runtime_context_content_replacements_tool_call
    ON runtime_context_content_replacements (tool_call_id);

CREATE UNIQUE INDEX idx_runtime_context_replacements_stable_key
    ON runtime_context_content_replacements (session_id, tool_call_id, kind);

CREATE INDEX idx_runtime_context_projection_messages_projection_sequence
    ON runtime_context_projection_messages (projection_id, sequence);

CREATE INDEX idx_runtime_context_projections_session_created_at
    ON runtime_context_projections (session_id, created_at);

CREATE INDEX idx_runtime_context_projections_turn_step
    ON runtime_context_projections (turn_id, step);

CREATE INDEX idx_runtime_context_reactive_attempts_turn_attempt
    ON runtime_context_reactive_attempts (turn_id, attempt);

CREATE INDEX idx_runtime_context_read_state_snapshots_projection
    ON runtime_context_read_state_snapshots (projection_id);

CREATE INDEX idx_runtime_context_reinjections_projection
    ON runtime_context_reinjections (projection_id);

CREATE INDEX idx_runtime_context_snip_boundaries_projection
    ON runtime_context_snip_boundaries (projection_id);

CREATE INDEX idx_runtime_context_warnings_projection
    ON runtime_context_warnings (projection_id);

CREATE INDEX idx_runtime_events_session_sequence
    ON runtime_events (session_id, sequence);

CREATE INDEX idx_runtime_events_tool_call_sequence
    ON runtime_events (tool_call_id, sequence);

CREATE INDEX idx_runtime_events_turn_sequence
    ON runtime_events (turn_id, sequence);

CREATE INDEX idx_runtime_events_type_sequence
    ON runtime_events (type, sequence);

CREATE UNIQUE INDEX idx_runtime_events_message_completed_once
    ON runtime_events (message_id)
    WHERE type = 'message.completed' AND message_id IS NOT NULL;

CREATE INDEX idx_runtime_hook_executions_event_started_at
    ON runtime_hook_executions (event, started_at);

CREATE INDEX idx_runtime_hook_executions_session_started_at
    ON runtime_hook_executions (session_id, started_at);

CREATE INDEX idx_runtime_hook_executions_task_started_at
    ON runtime_hook_executions (task_id, started_at);

CREATE INDEX idx_runtime_hook_executions_tool_call_started_at
    ON runtime_hook_executions (tool_call_id, started_at);

CREATE INDEX idx_runtime_hook_executions_turn_started_at
    ON runtime_hook_executions (turn_id, started_at);

CREATE INDEX idx_runtime_mcp_requests_kind_status_created_at
    ON runtime_mcp_requests (kind, status, created_at);

CREATE INDEX idx_runtime_mcp_requests_server_status_created_at
    ON runtime_mcp_requests (server, status, created_at);

CREATE INDEX idx_runtime_mcp_requests_status_created_at
    ON runtime_mcp_requests (status, created_at);

CREATE INDEX idx_runtime_mcp_requests_turn_created_at
    ON runtime_mcp_requests (turn_id, created_at);

CREATE INDEX idx_runtime_permission_requests_status_created_at
    ON runtime_permission_requests (status, created_at);

CREATE INDEX idx_runtime_permission_requests_tool_call_id
    ON runtime_permission_requests (tool_call_id);

CREATE INDEX idx_runtime_permission_requests_turn_created_at
    ON runtime_permission_requests (turn_id, created_at);

CREATE INDEX idx_runtime_prompt_assemblies_projection
    ON runtime_prompt_assemblies (projection_id);

CREATE INDEX idx_runtime_prompt_assemblies_session_created_at
    ON runtime_prompt_assemblies (session_id, created_at);

CREATE INDEX idx_runtime_prompt_assemblies_turn_step
    ON runtime_prompt_assemblies (turn_id, step);

CREATE INDEX idx_runtime_recovery_links_resumed_turn ON runtime_recovery_links(resumed_turn_id);

CREATE INDEX idx_runtime_recovery_links_source_turn ON runtime_recovery_links(source_turn_id);

CREATE INDEX idx_objects_kind_created_at
    ON objects (kind, created_at);

CREATE INDEX idx_objects_project_created_at
    ON objects (project_id, created_at);

CREATE INDEX idx_objects_session_created_at
    ON objects (session_id, created_at);

CREATE INDEX idx_objects_task_created_at
    ON objects (task_id, created_at);

CREATE INDEX idx_objects_tool_call_created_at
    ON objects (tool_call_id, created_at);

CREATE INDEX idx_objects_turn_created_at
    ON objects (turn_id, created_at);

CREATE INDEX idx_runtime_run_checkpoints_run_created_at
    ON runtime_run_checkpoints (run_id, created_at);

CREATE INDEX idx_runtime_run_checkpoints_session_id
    ON runtime_run_checkpoints (session_id);

CREATE INDEX idx_runtime_run_sessions_session_id
    ON runtime_run_sessions (session_id);

CREATE INDEX idx_runtime_run_transitions_run_created_at
    ON runtime_run_transitions (run_id, created_at);

CREATE INDEX idx_runtime_run_transitions_session_created_at
    ON runtime_run_transitions (session_id, created_at);

CREATE INDEX idx_runtime_run_transitions_turn_created_at
    ON runtime_run_transitions (turn_id, created_at);

CREATE INDEX idx_runtime_runs_primary_session_id
    ON runtime_runs (primary_session_id);

CREATE INDEX idx_runtime_runs_workspace_updated_at
    ON runtime_runs (workspace_id, updated_at);

CREATE INDEX idx_runtime_sandbox_decisions_session_created_at
    ON runtime_sandbox_decisions (session_id, created_at);

CREATE INDEX idx_runtime_sandbox_decisions_task_created_at
    ON runtime_sandbox_decisions (task_id, created_at);

CREATE INDEX idx_runtime_sandbox_decisions_tool_call_created_at
    ON runtime_sandbox_decisions (tool_call_id, created_at);

CREATE INDEX idx_runtime_sandbox_decisions_turn_created_at
    ON runtime_sandbox_decisions (turn_id, created_at);

CREATE INDEX idx_runtime_tool_calls_session_started_at
    ON runtime_tool_calls (session_id, started_at);

CREATE INDEX idx_runtime_tool_calls_turn_started_at
    ON runtime_tool_calls (turn_id, started_at);

CREATE INDEX idx_runtime_turns_session_updated_at
    ON runtime_turns (session_id, updated_at);

CREATE INDEX idx_runtime_turns_status_updated_at
    ON runtime_turns (status, updated_at);

CREATE INDEX idx_runtime_user_inputs_session_created_at
    ON runtime_user_inputs (session_id, created_at);

CREATE INDEX idx_runtime_user_inputs_turn_id
    ON runtime_user_inputs (turn_id);

CREATE INDEX idx_runtime_worktrees_session_updated_at
    ON runtime_worktrees (session_id, updated_at);

CREATE INDEX idx_runtime_worktrees_status_updated_at
    ON runtime_worktrees (status, updated_at);

CREATE INDEX idx_runtime_worktrees_task_updated_at
    ON runtime_worktrees (task_id, updated_at);

CREATE INDEX idx_runtime_worktrees_turn_updated_at
    ON runtime_worktrees (turn_id, updated_at);

CREATE UNIQUE INDEX idx_runtime_worktrees_worktree_path
    ON runtime_worktrees (worktree_path);

CREATE INDEX idx_selected_models_configured_provider
    ON selected_models (configured_provider_id);

CREATE UNIQUE INDEX idx_selected_models_scope
    ON selected_models (
        scope,
        COALESCE(project_id, ''),
        COALESCE(session_id, '')
    );

CREATE INDEX idx_sessions_created_at ON sessions (created_at);

CREATE INDEX idx_sessions_project_active
ON sessions(project_id, deleted_at, updated_at DESC);

CREATE INDEX idx_sessions_project_active_recent
ON sessions(project_id, deleted_at, pinned DESC, updated_at DESC);

CREATE INDEX idx_sessions_scope_active
ON sessions(scope, deleted_at, updated_at DESC);

CREATE INDEX idx_sessions_scope_project_id ON sessions (scope, project_id);

CREATE INDEX idx_sessions_standalone_active_recent
ON sessions(scope, deleted_at, pinned DESC, updated_at DESC)
WHERE scope = 'standalone' AND project_id IS NULL;

CREATE INDEX idx_sessions_active_recent
ON sessions(deleted_at, pinned DESC, updated_at DESC);

CREATE VIRTUAL TABLE message_search_fts USING fts5(
    message_id UNINDEXED,
    session_id UNINDEXED,
    role UNINDEXED,
    content,
    created_at UNINDEXED
);

CREATE TRIGGER update_files_updated_at
AFTER UPDATE ON files
BEGIN
UPDATE files SET updated_at = strftime('%s', 'now')
WHERE id = new.id;
END;

CREATE TRIGGER update_messages_updated_at
AFTER UPDATE ON messages
BEGIN
UPDATE messages SET updated_at = strftime('%s', 'now')
WHERE id = new.id;
END;

CREATE TRIGGER message_search_fts_insert
AFTER INSERT ON messages
BEGIN
INSERT INTO message_search_fts (message_id, session_id, role, content, created_at)
VALUES (new.id, new.session_id, new.role, new.parts, new.created_at);
END;

CREATE TRIGGER message_search_fts_update
AFTER UPDATE ON messages
BEGIN
DELETE FROM message_search_fts WHERE message_id = old.id;
INSERT INTO message_search_fts (message_id, session_id, role, content, created_at)
VALUES (new.id, new.session_id, new.role, new.parts, new.created_at);
END;

CREATE TRIGGER message_search_fts_delete
AFTER DELETE ON messages
BEGIN
DELETE FROM message_search_fts WHERE message_id = old.id;
END;

CREATE TRIGGER update_session_message_count_on_delete
AFTER DELETE ON messages
BEGIN
UPDATE sessions SET
    message_count = message_count - 1
WHERE id = old.session_id;
END;

CREATE TRIGGER update_session_message_count_on_insert
AFTER INSERT ON messages
BEGIN
UPDATE sessions SET
    message_count = message_count + 1
WHERE id = new.session_id;
END;

CREATE TRIGGER update_sessions_updated_at
AFTER UPDATE ON sessions
BEGIN
UPDATE sessions SET updated_at = strftime('%s', 'now')
WHERE id = new.id;
END;

INSERT INTO runtime_settings (key, value, updated_at) VALUES ('schema_generation', '2', strftime('%s', 'now'));
INSERT INTO runtime_settings (key, value, updated_at) VALUES ('delete_mode', 'hard', strftime('%s', 'now'));
