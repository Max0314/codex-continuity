use aes_gcm::{
    aead::{Aead, KeyInit},
    Aes256Gcm, Nonce,
};
use base64::{engine::general_purpose::STANDARD_NO_PAD, Engine as _};
use chrono::{DateTime, Duration as ChronoDuration, Utc};
use rand::RngCore;
use reqwest::StatusCode;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{
    collections::HashMap,
    fs,
    io::{BufRead, BufReader, Read, Write},
    path::{Path, PathBuf},
    sync::Arc,
    time::{Duration, Instant, SystemTime},
};
use tauri::{
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    AppHandle, Manager, PhysicalPosition, Position, State, WebviewUrl, WebviewWindowBuilder,
    WindowEvent,
};
use tauri_plugin_autostart::{MacosLauncher, ManagerExt as AutostartExt};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};
use tauri_plugin_shell::ShellExt;

const APP_VERSION: &str = env!("CARGO_PKG_VERSION");
const KEYRING_SERVICE: &str = "com.neonet.codexcontinuity";
const AUTO_SYNC_INTERVAL_SECONDS: i64 = 300;
const MAX_LOCAL_CONVERSATIONS: usize = 1000;
const MAX_BUNDLE_MIB: u16 = 500;

#[derive(Clone, Default)]
struct AppRuntime {
    sync_lock: Arc<tokio::sync::Mutex<()>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct SettingsFile {
    server_url: String,
    root: String,
    device_name: String,
    device_id: String,
    auto_sync: bool,
    launch_at_startup: bool,
    theme: String,
    sync_days: u16,
    selected_projects: Vec<String>,
    include_archived: bool,
    max_bundle_mib: u16,
}

impl Default for SettingsFile {
    fn default() -> Self {
        let host = hostname::get()
            .ok()
            .and_then(|value| value.into_string().ok())
            .filter(|value| !value.trim().is_empty())
            .unwrap_or_else(|| "Windows 电脑".to_string());
        Self {
            server_url: String::new(),
            root: r"D:\code_CPL".to_string(),
            device_name: host,
            device_id: String::new(),
            auto_sync: true,
            launch_at_startup: true,
            theme: "blue".to_string(),
            sync_days: 7,
            selected_projects: Vec::new(),
            include_archived: false,
            max_bundle_mib: MAX_BUNDLE_MIB,
        }
    }
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct PublicSettings {
    server_url: String,
    root: String,
    device_name: String,
    device_id: String,
    auto_sync: bool,
    launch_at_startup: bool,
    theme: String,
    sync_days: u16,
    selected_projects: Vec<String>,
    include_archived: bool,
    max_bundle_mib: u16,
    has_token: bool,
    has_encryption_key: bool,
    version: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SaveSettingsRequest {
    server_url: String,
    root: String,
    device_name: String,
    token: Option<String>,
    encryption_key: Option<String>,
    auto_sync: bool,
    launch_at_startup: bool,
    theme: String,
    sync_days: u16,
    selected_projects: Vec<String>,
    include_archived: bool,
    max_bundle_mib: u16,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct SaveSettingsResult {
    settings: PublicSettings,
    generated_key: Option<String>,
    connection: ConnectionResult,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct ConnectionResult {
    ok: bool,
    latency_ms: u128,
    service: String,
    checked_at: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct UploadTestResult {
    ok: bool,
    plaintext_bytes: usize,
    encrypted_bytes: usize,
    server_received_bytes: usize,
    latency_ms: u128,
    digest: String,
    discarded: bool,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct Manifest {
    device_name: String,
    root_name: String,
    workspace_key: String,
    projects: Vec<ManifestProject>,
    sessions: Vec<ManifestSession>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct ManifestProject {
    name: String,
    relative_path: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct ManifestSession {
    id: String,
    thread_name: String,
    relative_cwd: String,
    modified_at: String,
    archived: bool,
    archive_path: String,
    size: i64,
    included: bool,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Handoff {
    id: String,
    source_device_name: String,
    status: String,
    manifest: Option<Manifest>,
    blob_size: i64,
    created_at: String,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct Conversation {
    id: String,
    title: String,
    preview: String,
    project_name: String,
    relative_cwd: String,
    updated_at: String,
    source_device_name: String,
    source_device_os: String,
    current_device: bool,
    local: bool,
    archived: bool,
    sync_status: String,
    size: i64,
    handoff_id: Option<String>,
    continuation_mode: String,
    archive_path: Option<String>,
}

#[derive(Debug, Clone)]
struct LocalConversation {
    public: Conversation,
    source_path: PathBuf,
    modified_millis: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct SyncStateFile {
    last_success_at: Option<String>,
    last_attempt_at: Option<String>,
    next_sync_at: Option<String>,
    last_error: Option<String>,
    last_fingerprint: String,
}

impl Default for SyncStateFile {
    fn default() -> Self {
        Self {
            last_success_at: None,
            last_attempt_at: None,
            next_sync_at: Some(
                (Utc::now() + ChronoDuration::seconds(AUTO_SYNC_INTERVAL_SECONDS)).to_rfc3339(),
            ),
            last_error: None,
            last_fingerprint: String::new(),
        }
    }
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct SyncRuntime {
    phase: String,
    last_success_at: Option<String>,
    last_attempt_at: Option<String>,
    next_sync_at: Option<String>,
    last_error: Option<String>,
    pending_uploads: usize,
    progress: u8,
    scanned_conversations: usize,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct ActivityItem {
    id: String,
    kind: String,
    title: String,
    detail: String,
    tone: String,
    time: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct DashboardSnapshot {
    configured: bool,
    settings: PublicSettings,
    connection: Option<ConnectionResult>,
    conversations: Vec<Conversation>,
    sync_projects: Vec<SyncProjectOption>,
    sync: SyncRuntime,
    activities: Vec<ActivityItem>,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct SyncProjectOption {
    relative_path: String,
    name: String,
    conversation_count: usize,
    total_bytes: i64,
    last_updated_at: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ActionResult {
    ok: bool,
    message: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ContinueResult {
    ok: bool,
    message: String,
    mode: String,
    session_id: String,
    workspace_path: String,
    handoff_path: Option<String>,
    prompt: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ArchiveResult {
    ok: bool,
    message: String,
    path: Option<String>,
}

#[derive(Debug, Deserialize)]
struct DeviceEnvelope {
    device: DevicePayload,
}

#[derive(Debug, Deserialize)]
struct DevicePayload {
    id: String,
}

#[derive(Debug, Deserialize)]
struct HandoffsEnvelope {
    handoffs: Vec<Handoff>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct UploadTestEnvelope {
    received_bytes: usize,
    sha256: String,
    discarded: bool,
}

fn settings_path(app: &AppHandle) -> Result<PathBuf, String> {
    app.path()
        .app_config_dir()
        .map(|path| path.join("settings.json"))
        .map_err(|error| format!("无法确定配置目录：{error}"))
}

fn sync_state_path(app: &AppHandle) -> Result<PathBuf, String> {
    app.path()
        .app_config_dir()
        .map(|path| path.join("sync-state.json"))
        .map_err(|error| format!("无法确定同步状态目录：{error}"))
}

fn outbox_path(app: &AppHandle) -> Result<PathBuf, String> {
    app.path()
        .app_config_dir()
        .map(|path| path.join("outbox"))
        .map_err(|error| format!("无法确定离线队列目录：{error}"))
}

fn load_settings_file(app: &AppHandle) -> Result<SettingsFile, String> {
    let path = settings_path(app)?;
    if !path.exists() {
        return Ok(SettingsFile::default());
    }
    let raw = fs::read_to_string(&path).map_err(|error| format!("读取配置失败：{error}"))?;
    let mut settings =
        serde_json::from_str::<SettingsFile>(&raw).map_err(|error| format!("配置文件无效：{error}"))?;
    if settings.max_bundle_mib == 0 {
        settings.max_bundle_mib = MAX_BUNDLE_MIB;
    }
    if ![0, 2, 5, 7].contains(&settings.sync_days) {
        settings.sync_days = 7;
    }
    Ok(settings)
}

fn write_settings_file(app: &AppHandle, settings: &SettingsFile) -> Result<(), String> {
    let path = settings_path(app)?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|error| format!("创建配置目录失败：{error}"))?;
    }
    let raw = serde_json::to_vec_pretty(settings).map_err(|error| error.to_string())?;
    fs::write(path, raw).map_err(|error| format!("保存配置失败：{error}"))
}

fn load_sync_state(app: &AppHandle) -> SyncStateFile {
    let Ok(path) = sync_state_path(app) else {
        return SyncStateFile::default();
    };
    let Ok(raw) = fs::read_to_string(path) else {
        return SyncStateFile::default();
    };
    serde_json::from_str(&raw).unwrap_or_default()
}

fn write_sync_state(app: &AppHandle, state: &SyncStateFile) -> Result<(), String> {
    let path = sync_state_path(app)?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|error| format!("创建同步状态目录失败：{error}"))?;
    }
    let raw = serde_json::to_vec_pretty(state).map_err(|error| error.to_string())?;
    fs::write(path, raw).map_err(|error| format!("保存同步状态失败：{error}"))
}

fn secret_entry(name: &str) -> Result<keyring::Entry, String> {
    keyring::Entry::new(KEYRING_SERVICE, name)
        .map_err(|error| format!("Windows 凭据存储不可用：{error}"))
}

fn read_secret(name: &str) -> Option<String> {
    secret_entry(name).ok()?.get_password().ok()
}

fn write_secret(name: &str, value: &str) -> Result<(), String> {
    secret_entry(name)?
        .set_password(value)
        .map_err(|error| format!("保存 Windows 凭据失败：{error}"))
}

fn public_settings(settings: &SettingsFile) -> PublicSettings {
    PublicSettings {
        server_url: settings.server_url.clone(),
        root: settings.root.clone(),
        device_name: settings.device_name.clone(),
        device_id: settings.device_id.clone(),
        auto_sync: settings.auto_sync,
        launch_at_startup: settings.launch_at_startup,
        theme: settings.theme.clone(),
        sync_days: settings.sync_days,
        selected_projects: settings.selected_projects.clone(),
        include_archived: settings.include_archived,
        max_bundle_mib: settings.max_bundle_mib,
        has_token: read_secret("api-token").is_some(),
        has_encryption_key: read_secret("encryption-key").is_some(),
        version: APP_VERSION.to_string(),
    }
}

fn normalized_url(value: &str) -> Result<String, String> {
    let value = value.trim().trim_end_matches('/');
    if !(value.starts_with("http://") || value.starts_with("https://")) {
        return Err("服务端地址必须以 http:// 或 https:// 开头".to_string());
    }
    Ok(value.to_string())
}

fn now_iso() -> String {
    Utc::now().to_rfc3339()
}

fn system_time_iso(value: SystemTime) -> String {
    DateTime::<Utc>::from(value).to_rfc3339()
}

fn response_error(status: StatusCode, body: &str) -> String {
    serde_json::from_str::<Value>(body)
        .ok()
        .and_then(|value| value.get("error")?.as_str().map(str::to_string))
        .unwrap_or_else(|| format!("服务端返回 {status}"))
}

fn configured(settings: &SettingsFile) -> bool {
    !settings.server_url.is_empty()
        && !settings.device_id.is_empty()
        && Path::new(&settings.root).is_dir()
        && read_secret("api-token").is_some()
        && read_secret("encryption-key").is_some()
}

async fn check_connection(server_url: &str) -> Result<ConnectionResult, String> {
    let server_url = normalized_url(server_url)?;
    let started = Instant::now();
    let response = reqwest::Client::builder()
        .timeout(Duration::from_secs(8))
        .build()
        .map_err(|error| error.to_string())?
        .get(format!("{server_url}/api/v1/health"))
        .send()
        .await
        .map_err(|error| format!("无法连接服务端：{error}"))?;
    let latency_ms = started.elapsed().as_millis();
    if !response.status().is_success() {
        let status = response.status();
        let body = response.text().await.unwrap_or_default();
        return Err(response_error(status, &body));
    }
    let payload = response.json::<Value>().await.unwrap_or_default();
    Ok(ConnectionResult {
        ok: true,
        latency_ms,
        service: payload
            .get("service")
            .and_then(Value::as_str)
            .unwrap_or("codex-continuity")
            .to_string(),
        checked_at: now_iso(),
    })
}

async fn list_handoffs(settings: &SettingsFile, token: &str) -> Result<Vec<Handoff>, String> {
    let response = reqwest::Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .map_err(|error| error.to_string())?
        .get(format!(
            "{}/api/v1/client/handoffs",
            settings.server_url.trim_end_matches('/')
        ))
        .query(&[("target", settings.device_name.as_str())])
        .bearer_auth(token)
        .send()
        .await
        .map_err(|error| format!("读取云端会话快照失败：{error}"))?;
    let status = response.status();
    let body = response.text().await.unwrap_or_default();
    if !status.is_success() {
        return Err(response_error(status, &body));
    }
    serde_json::from_str::<HandoffsEnvelope>(&body)
        .map(|value| value.handoffs)
        .map_err(|error| format!("云端快照格式无效：{error}"))
}

fn codex_root() -> Option<PathBuf> {
    dirs::home_dir().map(|home| home.join(".codex"))
}

fn read_thread_names(path: &Path) -> HashMap<String, String> {
    let mut names = HashMap::new();
    let Ok(file) = fs::File::open(path) else {
        return names;
    };
    for line in BufReader::new(file).lines().map_while(Result::ok) {
        let Ok(value) = serde_json::from_str::<Value>(&line) else {
            continue;
        };
        let Some(id) = value.get("id").and_then(Value::as_str) else {
            continue;
        };
        let Some(title) = value.get("thread_name").and_then(Value::as_str) else {
            continue;
        };
        if !title.trim().is_empty() {
            names.insert(id.to_string(), title.trim().to_string());
        }
    }
    names
}

fn read_session_meta(path: &Path) -> Option<(String, PathBuf)> {
    let file = fs::File::open(path).ok()?;
    let mut reader = BufReader::new(file.take(4 * 1024 * 1024));
    let mut line = String::new();
    reader.read_line(&mut line).ok()?;
    let value = serde_json::from_str::<Value>(&line).ok()?;
    let payload = value.get("payload")?;
    let id = payload
        .get("session_id")
        .or_else(|| payload.get("id"))
        .and_then(Value::as_str)?
        .to_string();
    let cwd = payload.get("cwd").and_then(Value::as_str)?;
    Some((id, PathBuf::from(cwd)))
}

fn is_under_root(path: &Path, root: &Path) -> bool {
    let Ok(path) = path.canonicalize() else {
        return false;
    };
    let Ok(root) = root.canonicalize() else {
        return false;
    };
    path.starts_with(root)
}

fn project_name(relative_cwd: &Path, root: &Path) -> String {
    relative_cwd
        .components()
        .next()
        .map(|value| value.as_os_str().to_string_lossy().to_string())
        .filter(|value| value != ".")
        .or_else(|| {
            root.file_name()
                .map(|value| value.to_string_lossy().to_string())
        })
        .unwrap_or_else(|| "code_CPL".to_string())
}

fn project_relative_path(relative_cwd: &Path) -> String {
    relative_cwd
        .components()
        .next()
        .map(|value| value.as_os_str().to_string_lossy().replace('\\', "/"))
        .filter(|value| !value.is_empty() && value != ".")
        .unwrap_or_else(|| ".".to_string())
}

fn walk_session_files(root: &Path, output: &mut Vec<PathBuf>) {
    let Ok(entries) = fs::read_dir(root) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if path.is_dir() {
            walk_session_files(&path, output);
        } else if path
            .extension()
            .and_then(|value| value.to_str())
            .is_some_and(|value| value.eq_ignore_ascii_case("jsonl"))
        {
            output.push(path);
        }
    }
}

fn scan_local_conversations(settings: &SettingsFile) -> Vec<LocalConversation> {
    let root = PathBuf::from(&settings.root);
    let Some(codex_root) = codex_root() else {
        return Vec::new();
    };
    let names = read_thread_names(&codex_root.join("session_index.jsonl"));
    let mut active_paths = Vec::new();
    walk_session_files(&codex_root.join("sessions"), &mut active_paths);
    let mut candidates = active_paths
        .into_iter()
        .map(|path| (path, false))
        .collect::<Vec<_>>();
    if settings.include_archived {
        let mut archived_paths = Vec::new();
        walk_session_files(&codex_root.join("archived_sessions"), &mut archived_paths);
        candidates.extend(archived_paths.into_iter().map(|path| (path, true)));
    }
    let mut by_id = HashMap::<String, LocalConversation>::new();
    for (path, archived) in candidates {
        let item = (|| {
            let (id, cwd) = read_session_meta(&path)?;
            if id.trim().is_empty() || !is_under_root(&cwd, &root) {
                return None;
            }
            let metadata = fs::metadata(&path).ok()?;
            let modified = metadata.modified().unwrap_or(SystemTime::UNIX_EPOCH);
            let modified_millis = modified
                .duration_since(SystemTime::UNIX_EPOCH)
                .unwrap_or_default()
                .as_millis() as i64;
            let relative = cwd.strip_prefix(&root).unwrap_or(&cwd);
            let relative_cwd = relative.to_string_lossy().replace('\\', "/");
            let project = project_name(relative, &root);
            let title = names
                .get(&id)
                .cloned()
                .filter(|value| !value.trim().is_empty())
                .unwrap_or_else(|| format!("Codex 会话 {}", &id[..id.len().min(8)]));
            Some(LocalConversation {
                public: Conversation {
                    id: id.clone(),
                    title,
                    preview: format!("{} · 本机 Codex 原始会话", project),
                    project_name: project,
                    relative_cwd,
                    updated_at: system_time_iso(modified),
                    source_device_name: settings.device_name.clone(),
                    source_device_os: "Windows".to_string(),
                    current_device: true,
                    local: true,
                    archived,
                    sync_status: "local".to_string(),
                    size: metadata.len() as i64,
                    handoff_id: None,
                    continuation_mode: "native-local".to_string(),
                    archive_path: Some(path.to_string_lossy().to_string()),
                },
                source_path: path,
                modified_millis,
            })
        })();
        if let Some(item) = item {
            if archived && by_id.contains_key(&item.public.id) {
                continue;
            }
            by_id.insert(item.public.id.clone(), item);
        }
    }
    let mut conversations = by_id.into_values().collect::<Vec<_>>();
    conversations.sort_by(|left, right| right.modified_millis.cmp(&left.modified_millis));
    conversations.truncate(MAX_LOCAL_CONVERSATIONS);
    conversations
}

fn scoped_local_conversations(
    settings: &SettingsFile,
    conversations: &[LocalConversation],
) -> Vec<LocalConversation> {
    let cutoff = if settings.sync_days == 0 {
        None
    } else {
        Some(
            (Utc::now() - ChronoDuration::days(settings.sync_days as i64)).timestamp_millis(),
        )
    };
    let selected = settings
        .selected_projects
        .iter()
        .map(|path| path.trim_matches('/').replace('\\', "/").to_lowercase())
        .collect::<Vec<_>>();
    conversations
        .iter()
        .filter(|item| {
            cutoff.is_none_or(|value| item.modified_millis >= value)
                && (selected.is_empty()
                    || selected.iter().any(|project| {
                        let path = item.public.relative_cwd.to_lowercase();
                        (project == "." && path == ".")
                            || path == *project
                            || path.starts_with(&format!("{project}/"))
                    }))
        })
        .cloned()
        .collect()
}

fn sync_project_options(
    settings: &SettingsFile,
    conversations: &[LocalConversation],
) -> Vec<SyncProjectOption> {
    let root = PathBuf::from(&settings.root);
    let root_name = root
        .file_name()
        .map(|value| value.to_string_lossy().to_string())
        .unwrap_or_else(|| "工作区根目录".to_string());
    let mut grouped = HashMap::<String, SyncProjectOption>::new();
    for item in conversations {
        let relative_path = project_relative_path(Path::new(&item.public.relative_cwd));
        let entry = grouped
            .entry(relative_path.clone())
            .or_insert_with(|| SyncProjectOption {
                relative_path: relative_path.clone(),
                name: if relative_path == "." {
                    root_name.clone()
                } else {
                    relative_path
                        .split('/')
                        .next_back()
                        .unwrap_or(&relative_path)
                        .to_string()
                },
                conversation_count: 0,
                total_bytes: 0,
                last_updated_at: item.public.updated_at.clone(),
            });
        entry.conversation_count += 1;
        entry.total_bytes += item.public.size;
        if item.public.updated_at > entry.last_updated_at {
            entry.last_updated_at = item.public.updated_at.clone();
        }
    }
    let mut result = grouped.into_values().collect::<Vec<_>>();
    result.sort_by(|left, right| {
        right
            .last_updated_at
            .cmp(&left.last_updated_at)
            .then_with(|| left.name.cmp(&right.name))
    });
    result
}

fn parse_time(value: &str) -> i64 {
    DateTime::parse_from_rfc3339(value)
        .map(|time| time.timestamp_millis())
        .unwrap_or_default()
}

fn merge_conversations(
    settings: &SettingsFile,
    local: Vec<LocalConversation>,
    handoffs: &[Handoff],
    pending_uploads: usize,
) -> Vec<Conversation> {
    let mut result = local
        .iter()
        .map(|item| (item.public.id.clone(), item.public.clone()))
        .collect::<HashMap<_, _>>();

    let mut ordered_handoffs = handoffs.to_vec();
    ordered_handoffs.sort_by(|left, right| right.created_at.cmp(&left.created_at));
    for handoff in ordered_handoffs {
        let Some(manifest) = handoff.manifest.as_ref() else {
            continue;
        };
        let project_by_path = manifest
            .projects
            .iter()
            .map(|project| {
                (
                    project.relative_path.to_lowercase().replace('\\', "/"),
                    project.name.clone(),
                )
            })
            .collect::<Vec<_>>();
        for session in &manifest.sessions {
            if session.id.trim().is_empty() {
                continue;
            }
            let remote_updated = parse_time(&session.modified_at);
            if let Some(existing) = result.get_mut(&session.id) {
                if remote_updated >= parse_time(&existing.updated_at) {
                    existing.sync_status = "synced".to_string();
                    existing.handoff_id = Some(handoff.id.clone());
                    existing.source_device_name = handoff.source_device_name.clone();
                    existing.archive_path = if session.archive_path.is_empty() {
                        existing.archive_path.clone()
                    } else {
                        Some(session.archive_path.clone())
                    };
                } else if pending_uploads > 0 {
                    existing.sync_status = "queued".to_string();
                }
                continue;
            }
            let relative_cwd = session.relative_cwd.replace('\\', "/");
            let project = project_by_path
                .iter()
                .filter(|(path, _)| {
                    relative_cwd.to_lowercase() == *path
                        || relative_cwd.to_lowercase().starts_with(&format!("{path}/"))
                })
                .max_by_key(|(path, _)| path.len())
                .map(|(_, name)| name.clone())
                .or_else(|| {
                    Path::new(&relative_cwd)
                        .components()
                        .next()
                        .map(|value| value.as_os_str().to_string_lossy().to_string())
                })
                .unwrap_or_else(|| manifest.root_name.clone());
            let title = if session.thread_name.trim().is_empty() {
                format!("Codex 会话 {}", &session.id[..session.id.len().min(8)])
            } else {
                session.thread_name.clone()
            };
            result.insert(
                session.id.clone(),
                Conversation {
                    id: session.id.clone(),
                    title,
                    preview: format!("{} · 来自 {}", project, handoff.source_device_name),
                    project_name: project,
                    relative_cwd,
                    updated_at: session.modified_at.clone(),
                    source_device_name: handoff.source_device_name.clone(),
                    source_device_os: "Windows".to_string(),
                    current_device: handoff
                        .source_device_name
                        .eq_ignore_ascii_case(&settings.device_name),
                    local: false,
                    archived: session.archived,
                    sync_status: if handoff.status == "pending" {
                        "available"
                    } else {
                        "imported"
                    }
                    .to_string(),
                    size: session.size,
                    handoff_id: Some(handoff.id.clone()),
                    continuation_mode: "context".to_string(),
                    archive_path: if session.archive_path.is_empty() {
                        None
                    } else {
                        Some(session.archive_path.clone())
                    },
                },
            );
        }
    }
    let mut conversations = result.into_values().collect::<Vec<_>>();
    conversations.sort_by(|left, right| {
        parse_time(&right.updated_at)
            .cmp(&parse_time(&left.updated_at))
            .then_with(|| left.title.cmp(&right.title))
    });
    conversations
}

fn conversation_fingerprint(local: &[LocalConversation]) -> String {
    let mut hasher = Sha256::new();
    for item in local {
        hasher.update(item.public.id.as_bytes());
        hasher.update(item.modified_millis.to_le_bytes());
        hasher.update(item.public.size.to_le_bytes());
        hasher.update(item.source_path.to_string_lossy().as_bytes());
    }
    format!("{:x}", hasher.finalize())
}

fn outbox_count(app: &AppHandle) -> usize {
    let Ok(path) = outbox_path(app) else {
        return 0;
    };
    fs::read_dir(path)
        .ok()
        .into_iter()
        .flatten()
        .flatten()
        .filter(|entry| {
            entry
                .path()
                .extension()
                .and_then(|value| value.to_str())
                .is_some_and(|value| value.eq_ignore_ascii_case("ccx"))
        })
        .count()
}

fn activities_for(
    settings: &SettingsFile,
    state: &SyncStateFile,
    handoffs: &[Handoff],
    pending_uploads: usize,
) -> Vec<ActivityItem> {
    let mut activities = Vec::new();
    if let Some(error) = &state.last_error {
        activities.push(ActivityItem {
            id: "sync-error".to_string(),
            kind: "sync".to_string(),
            title: "最近一次同步未完成".to_string(),
            detail: error.clone(),
            tone: "error".to_string(),
            time: state.last_attempt_at.clone().unwrap_or_else(now_iso),
        });
    } else if let Some(time) = &state.last_success_at {
        activities.push(ActivityItem {
            id: "sync-success".to_string(),
            kind: "sync".to_string(),
            title: "会话快照同步完成".to_string(),
            detail: if pending_uploads == 0 {
                "云端与本机已确认到当前版本".to_string()
            } else {
                format!("{pending_uploads} 个快照等待网络恢复后上传")
            },
            tone: if pending_uploads == 0 {
                "success"
            } else {
                "warning"
            }
            .to_string(),
            time: time.clone(),
        });
    }
    for handoff in handoffs.iter().take(4) {
        let source_is_current = handoff
            .source_device_name
            .eq_ignore_ascii_case(&settings.device_name);
        let project_count = handoff
            .manifest
            .as_ref()
            .map(|manifest| manifest.projects.len())
            .unwrap_or_default();
        let session_count = handoff
            .manifest
            .as_ref()
            .map(|manifest| manifest.sessions.len())
            .unwrap_or_default();
        activities.push(ActivityItem {
            id: handoff.id.clone(),
            kind: if source_is_current {
                "upload"
            } else {
                "archive"
            }
            .to_string(),
            title: if source_is_current {
                "本机已发布加密快照"
            } else {
                "收到其他设备的会话快照"
            }
            .to_string(),
            detail: format!(
                "{} · {} 个项目 / {} 条会话 · {}",
                handoff.source_device_name,
                project_count,
                session_count,
                human_bytes(handoff.blob_size)
            ),
            tone: "info".to_string(),
            time: handoff.created_at.clone(),
        });
    }
    if activities.is_empty() {
        activities.push(ActivityItem {
            id: "ready".to_string(),
            kind: "scan".to_string(),
            title: "Codex Continuity 已就绪".to_string(),
            detail: "完成配置后，会自动扫描工作根目录中的 Codex 会话".to_string(),
            tone: "info".to_string(),
            time: now_iso(),
        });
    }
    activities
}

fn human_bytes(size: i64) -> String {
    if size < 1024 {
        return format!("{} B", size.max(0));
    }
    let units = ["KB", "MB", "GB", "TB"];
    let mut value = size.max(0) as f64;
    let mut index = 0;
    while value >= 1024.0 && index < units.len() - 1 {
        value /= 1024.0;
        index += 1;
    }
    format!("{value:.1} {}", units[index])
}

async fn dashboard_data(app: &AppHandle) -> Result<DashboardSnapshot, String> {
    let settings = load_settings_file(app)?;
    let is_configured = configured(&settings);
    let local = scan_local_conversations(&settings);
    let scoped_local = scoped_local_conversations(&settings, &local);
    let sync_projects = sync_project_options(&settings, &local);
    let pending_uploads = outbox_count(app);
    let mut connection = None;
    let mut handoffs = Vec::new();
    if is_configured {
        if let Ok(result) = check_connection(&settings.server_url).await {
            connection = Some(result);
            if let Some(token) = read_secret("api-token") {
                handoffs = list_handoffs(&settings, &token).await.unwrap_or_default();
            }
        }
    }
    let state = load_sync_state(app);
    let conversations = merge_conversations(&settings, local, &handoffs, pending_uploads);
    let phase = if !settings.auto_sync {
        "paused"
    } else if state.last_error.is_some() && pending_uploads > 0 {
        "queued"
    } else {
        "idle"
    };
    Ok(DashboardSnapshot {
        configured: is_configured,
        settings: public_settings(&settings),
        connection,
        sync_projects,
        sync: SyncRuntime {
            phase: phase.to_string(),
            last_success_at: state.last_success_at.clone(),
            last_attempt_at: state.last_attempt_at.clone(),
            next_sync_at: state.next_sync_at.clone(),
            last_error: state.last_error.clone(),
            pending_uploads,
            progress: if state.last_error.is_some() { 0 } else { 100 },
            scanned_conversations: scoped_local.len(),
        },
        activities: activities_for(&settings, &state, &handoffs, pending_uploads),
        conversations,
    })
}

fn encrypted_test_payload(key: &[u8]) -> Result<(usize, Vec<u8>, String), String> {
    let cipher = Aes256Gcm::new_from_slice(key).map_err(|_| "加密密钥无效".to_string())?;
    let mut plaintext = vec![0u8; 64 * 1024];
    rand::rng().fill_bytes(&mut plaintext);
    let mut nonce_bytes = [0u8; 12];
    rand::rng().fill_bytes(&mut nonce_bytes);
    let ciphertext = cipher
        .encrypt(Nonce::from_slice(&nonce_bytes), plaintext.as_ref())
        .map_err(|_| "生成加密测试包失败".to_string())?;
    let mut encrypted = nonce_bytes.to_vec();
    encrypted.extend(ciphertext);
    let digest = format!("{:x}", Sha256::digest(&encrypted));
    Ok((plaintext.len(), encrypted, digest))
}

fn temp_core_config(settings: &SettingsFile) -> Result<tempfile::NamedTempFile, String> {
    let token = read_secret("api-token").ok_or_else(|| "请先保存 API 令牌".to_string())?;
    let encryption_key =
        read_secret("encryption-key").ok_or_else(|| "请先配置加密密钥".to_string())?;
    let mut file = tempfile::Builder::new()
        .prefix("codex-continuity-")
        .suffix(".json")
        .tempfile()
        .map_err(|error| format!("创建临时配置失败：{error}"))?;
    serde_json::to_writer_pretty(
        &mut file,
        &json!({
            "serverUrl": settings.server_url,
            "token": token,
            "root": settings.root,
            "deviceName": settings.device_name,
            "deviceId": settings.device_id,
            "encryptionKey": encryption_key,
            "syncScope": {
                "days": settings.sync_days,
                "projectPaths": settings.selected_projects,
                "includeArchived": settings.include_archived,
                "maxBundleMiB": settings.max_bundle_mib
            }
        }),
    )
    .map_err(|error| format!("生成运行配置失败：{error}"))?;
    file.flush().map_err(|error| error.to_string())?;
    Ok(file)
}

async fn run_core_output(app: &AppHandle, arguments: Vec<String>) -> Result<String, String> {
    let sidecar = app
        .shell()
        .sidecar("continuity-core")
        .map_err(|error| format!("无法启动同步核心：{error}"))?;
    let output = sidecar
        .args(arguments)
        .output()
        .await
        .map_err(|error| format!("同步核心执行失败：{error}"))?;
    let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
    let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
    if !output.status.success() {
        return Err(if stderr.is_empty() {
            if stdout.is_empty() {
                "同步核心返回失败".to_string()
            } else {
                stdout
            }
        } else {
            stderr
        });
    }
    Ok(stdout)
}

async fn perform_sync(
    app: &AppHandle,
    runtime: &AppRuntime,
    force: bool,
) -> Result<ActionResult, String> {
    let _guard = runtime.sync_lock.lock().await;
    let settings = load_settings_file(app)?;
    if !configured(&settings) {
        return Err("请先在“设置”中完成服务端、API 令牌和加密密钥配置".to_string());
    }
    let local = scan_local_conversations(&settings);
    let scoped_local = scoped_local_conversations(&settings, &local);
    let fingerprint = conversation_fingerprint(&scoped_local);
    let mut state = load_sync_state(app);
    let before_pending = outbox_count(app);
    let due = state
        .next_sync_at
        .as_deref()
        .map(parse_time)
        .is_none_or(|time| time <= Utc::now().timestamp_millis());
    if !force
        && !due
        && before_pending == 0
        && !state.last_fingerprint.is_empty()
        && state.last_fingerprint == fingerprint
    {
        return Ok(ActionResult {
            ok: true,
            message: "当前没有新的会话变化".to_string(),
        });
    }

    state.last_attempt_at = Some(now_iso());
    state.next_sync_at =
        Some((Utc::now() + ChronoDuration::seconds(AUTO_SYNC_INTERVAL_SECONDS)).to_rfc3339());
    state.last_error = None;
    write_sync_state(app, &state)?;

    let runtime_config = temp_core_config(&settings)?;
    let queue = outbox_path(app)?;
    fs::create_dir_all(&queue).map_err(|error| format!("创建离线队列失败：{error}"))?;

    if before_pending > 0 {
        let flush_result = run_core_output(
            app,
            vec![
                "flush".to_string(),
                "--config".to_string(),
                runtime_config.path().to_string_lossy().to_string(),
                "--queue-dir".to_string(),
                queue.to_string_lossy().to_string(),
            ],
        )
        .await;
        if let Err(error) = flush_result {
            state.last_error = Some(error.clone());
            write_sync_state(app, &state)?;
            return Err(error);
        }
    }

    if force || state.last_fingerprint != fingerprint {
        let publish_result = run_core_output(
            app,
            vec![
                "publish".to_string(),
                "--config".to_string(),
                runtime_config.path().to_string_lossy().to_string(),
                "--queue-dir".to_string(),
                queue.to_string_lossy().to_string(),
            ],
        )
        .await;
        if let Err(error) = publish_result {
            state.last_error = Some(error.clone());
            write_sync_state(app, &state)?;
            return Err(error);
        }
        state.last_fingerprint = fingerprint;
    }

    let after_pending = outbox_count(app);
    if after_pending == 0 {
        state.last_success_at = Some(now_iso());
        state.last_error = None;
    } else {
        state.last_error = Some(format!(
            "{after_pending} 个加密快照已保存在离线队列，网络恢复后会自动重试"
        ));
    }
    write_sync_state(app, &state)?;
    Ok(ActionResult {
        ok: true,
        message: if after_pending == 0 {
            format!("同步完成，已检查 {} 条范围内会话", scoped_local.len())
        } else {
            format!("{after_pending} 个加密快照已进入离线队列")
        },
    })
}

#[tauri::command]
fn get_settings(app: AppHandle) -> Result<PublicSettings, String> {
    load_settings_file(&app).map(|settings| public_settings(&settings))
}

#[tauri::command]
async fn save_settings(
    app: AppHandle,
    request: SaveSettingsRequest,
) -> Result<SaveSettingsResult, String> {
    let mut current = load_settings_file(&app)?;
    let server_url = normalized_url(&request.server_url)?;
    let root = PathBuf::from(request.root.trim());
    if !root.is_dir() {
        return Err("工作根目录不存在，请选择有效目录".to_string());
    }
    if request.device_name.trim().is_empty() {
        return Err("设备名称不能为空".to_string());
    }
    if !["blue", "teal", "violet"].contains(&request.theme.as_str()) {
        return Err("不支持的主题颜色".to_string());
    }
    if ![0, 2, 5, 7].contains(&request.sync_days) {
        return Err("同步时间范围只能是 2 天、5 天、7 天或不限制".to_string());
    }
    if request.max_bundle_mib == 0 || request.max_bundle_mib > MAX_BUNDLE_MIB {
        return Err(format!("单个加密同步包不能超过 {MAX_BUNDLE_MIB} MiB"));
    }
    let selected_projects = request
        .selected_projects
        .iter()
        .map(|path| path.trim().replace('\\', "/").trim_matches('/').to_string())
        .filter(|path| !path.is_empty())
        .collect::<Vec<_>>();
    if selected_projects
        .iter()
        .any(|path| path == ".." || path.starts_with("../") || Path::new(path).is_absolute())
    {
        return Err("同步项目路径无效".to_string());
    }
    let token = request
        .token
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
        .or_else(|| read_secret("api-token"))
        .ok_or_else(|| "请填写客户端 API 令牌".to_string())?;
    let existing_key = read_secret("encryption-key");
    let requested_key = request
        .encryption_key
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string);
    let generated_key = if requested_key.is_none() && existing_key.is_none() {
        let mut key = [0u8; 32];
        rand::rng().fill_bytes(&mut key);
        Some(STANDARD_NO_PAD.encode(key))
    } else {
        None
    };
    let encryption_key = requested_key
        .or(existing_key)
        .or_else(|| generated_key.clone())
        .ok_or_else(|| "无法生成加密密钥".to_string())?;
    let decoded = STANDARD_NO_PAD
        .decode(encryption_key.as_bytes())
        .map_err(|_| "加密密钥必须是 32 字节密钥的 Base64 文本".to_string())?;
    if decoded.len() != 32 {
        return Err("加密密钥必须是 32 字节密钥的 Base64 文本".to_string());
    }

    let connection = check_connection(&server_url).await?;
    let response = reqwest::Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .map_err(|error| error.to_string())?
        .post(format!("{server_url}/api/v1/client/devices"))
        .bearer_auth(&token)
        .json(&json!({
            "id": current.device_id,
            "name": request.device_name.trim(),
            "hostname": hostname::get().ok().map(|value| value.to_string_lossy().to_string()).unwrap_or_default(),
            "os": format!("windows/{}", std::env::consts::ARCH),
            "clientVersion": APP_VERSION,
            "lastSeenAt": "0001-01-01T00:00:00Z",
            "createdAt": "0001-01-01T00:00:00Z"
        }))
        .send()
        .await
        .map_err(|error| format!("注册设备失败：{error}"))?;
    let status = response.status();
    let body = response.text().await.unwrap_or_default();
    if !status.is_success() {
        return Err(response_error(status, &body));
    }
    let device = serde_json::from_str::<DeviceEnvelope>(&body)
        .map_err(|error| format!("设备注册响应无效：{error}"))?;

    write_secret("api-token", &token)?;
    write_secret("encryption-key", &encryption_key)?;
    current.server_url = server_url;
    current.root = root.to_string_lossy().to_string();
    current.device_name = request.device_name.trim().to_string();
    current.device_id = device.device.id;
    current.auto_sync = request.auto_sync;
    current.launch_at_startup = request.launch_at_startup;
    current.theme = request.theme;
    current.sync_days = request.sync_days;
    current.selected_projects = selected_projects;
    current.include_archived = request.include_archived;
    current.max_bundle_mib = request.max_bundle_mib;
    write_settings_file(&app, &current)?;
    if current.launch_at_startup {
        let _ = app.autolaunch().enable();
    } else {
        let _ = app.autolaunch().disable();
    }
    Ok(SaveSettingsResult {
        settings: public_settings(&current),
        generated_key,
        connection,
    })
}

#[tauri::command]
async fn test_connection(app: AppHandle) -> Result<ConnectionResult, String> {
    let settings = load_settings_file(&app)?;
    if settings.server_url.is_empty() {
        return Err("请先保存服务端配置".to_string());
    }
    check_connection(&settings.server_url).await
}

#[tauri::command]
async fn test_upload(app: AppHandle) -> Result<UploadTestResult, String> {
    let settings = load_settings_file(&app)?;
    let token = read_secret("api-token").ok_or_else(|| "请先保存 API 令牌".to_string())?;
    let key = read_secret("encryption-key").ok_or_else(|| "请先配置加密密钥".to_string())?;
    let key = STANDARD_NO_PAD
        .decode(key.as_bytes())
        .map_err(|_| "Windows 凭据中的加密密钥无效".to_string())?;
    let (plaintext_bytes, encrypted, local_digest) = encrypted_test_payload(&key)?;
    let started = Instant::now();
    let response = reqwest::Client::builder()
        .timeout(Duration::from_secs(20))
        .build()
        .map_err(|error| error.to_string())?
        .post(format!(
            "{}/api/v1/client/diagnostics/upload-test",
            settings.server_url.trim_end_matches('/')
        ))
        .bearer_auth(token)
        .header("Content-Type", "application/octet-stream")
        .body(encrypted.clone())
        .send()
        .await
        .map_err(|error| format!("上传测试失败：{error}"))?;
    let status = response.status();
    let body = response.text().await.unwrap_or_default();
    if !status.is_success() {
        return Err(response_error(status, &body));
    }
    let result = serde_json::from_str::<UploadTestEnvelope>(&body)
        .map_err(|error| format!("上传测试响应无效：{error}"))?;
    if result.sha256 != local_digest {
        return Err("上传测试包摘要不一致".to_string());
    }
    Ok(UploadTestResult {
        ok: true,
        plaintext_bytes,
        encrypted_bytes: encrypted.len(),
        server_received_bytes: result.received_bytes,
        latency_ms: started.elapsed().as_millis(),
        digest: result.sha256,
        discarded: result.discarded,
    })
}

#[tauri::command]
async fn get_dashboard(app: AppHandle) -> Result<DashboardSnapshot, String> {
    dashboard_data(&app).await
}

#[tauri::command]
async fn sync_now(app: AppHandle, runtime: State<'_, AppRuntime>) -> Result<ActionResult, String> {
    perform_sync(&app, runtime.inner(), true).await
}

#[tauri::command]
async fn continue_conversation(
    app: AppHandle,
    conversation_id: String,
) -> Result<ContinueResult, String> {
    let snapshot = dashboard_data(&app).await?;
    let conversation = snapshot
        .conversations
        .into_iter()
        .find(|item| item.id == conversation_id)
        .ok_or_else(|| "找不到这条会话，请刷新后重试".to_string())?;
    let settings = load_settings_file(&app)?;
    let workspace = PathBuf::from(&settings.root).join(&conversation.relative_cwd);
    if conversation.local {
        return Ok(ContinueResult {
            ok: true,
            message: "该会话已在本机，可直接在 Codex 中从任务列表继续".to_string(),
            mode: "native-local".to_string(),
            session_id: conversation.id,
            workspace_path: workspace.to_string_lossy().to_string(),
            handoff_path: None,
            prompt: Some(
                "这条任务已经存在于本机 Codex。打开对应工作区后，在任务列表中选择同名任务即可继续。"
                    .to_string(),
            ),
        });
    }
    let handoff_id = conversation
        .handoff_id
        .clone()
        .ok_or_else(|| "这条云端会话缺少快照标识".to_string())?;
    let runtime_config = temp_core_config(&settings)?;
    let output_dir = PathBuf::from(&settings.root)
        .join(".codex-continuity")
        .join("handoffs")
        .join(&handoff_id);
    let _ = run_core_output(
        &app,
        vec![
            "takeover".to_string(),
            "--config".to_string(),
            runtime_config.path().to_string_lossy().to_string(),
            "--id".to_string(),
            handoff_id.clone(),
            "--output".to_string(),
            output_dir.to_string_lossy().to_string(),
        ],
    )
    .await?;
    let handoff_path = output_dir.join("HANDOFF.md");
    Ok(ContinueResult {
        ok: true,
        message: "续接上下文已安全下载，可以在目标工作区继续".to_string(),
        mode: "context".to_string(),
        session_id: conversation.id,
        workspace_path: workspace.to_string_lossy().to_string(),
        handoff_path: Some(handoff_path.to_string_lossy().to_string()),
        prompt: Some(
            "请读取 HANDOFF.md 和 manifest.json，核对当前 Git 分支、提交、未完成事项和风险，然后从未完成事项继续。不要覆盖现有工作区。"
                .to_string(),
        ),
    })
}

#[tauri::command]
async fn export_archive(app: AppHandle, output: String) -> Result<ArchiveResult, String> {
    let output = PathBuf::from(output);
    if output.exists() {
        return Err("目标文件已存在，请选择新的文件名".to_string());
    }
    if let Some(parent) = output.parent() {
        fs::create_dir_all(parent).map_err(|error| format!("创建导出目录失败：{error}"))?;
    }
    let settings = load_settings_file(&app)?;
    let runtime_config = temp_core_config(&settings)?;
    run_core_output(
        &app,
        vec![
            "export".to_string(),
            "--config".to_string(),
            runtime_config.path().to_string_lossy().to_string(),
            "--output".to_string(),
            output.to_string_lossy().to_string(),
        ],
    )
    .await?;
    Ok(ArchiveResult {
        ok: true,
        message: "加密会话归档已导出".to_string(),
        path: Some(output.to_string_lossy().to_string()),
    })
}

#[tauri::command]
async fn import_archive(app: AppHandle, input: String) -> Result<ArchiveResult, String> {
    let input = PathBuf::from(input);
    if !input.is_file() {
        return Err("选择的归档文件不存在".to_string());
    }
    let settings = load_settings_file(&app)?;
    let runtime_config = temp_core_config(&settings)?;
    let output = PathBuf::from(&settings.root)
        .join(".codex-continuity")
        .join("imports")
        .join(Utc::now().format("%Y%m%d-%H%M%S").to_string());
    run_core_output(
        &app,
        vec![
            "import".to_string(),
            "--config".to_string(),
            runtime_config.path().to_string_lossy().to_string(),
            "--input".to_string(),
            input.to_string_lossy().to_string(),
            "--output".to_string(),
            output.to_string_lossy().to_string(),
        ],
    )
    .await?;
    Ok(ArchiveResult {
        ok: true,
        message: "归档已导入到只读续接目录".to_string(),
        path: Some(output.to_string_lossy().to_string()),
    })
}

#[tauri::command]
fn set_auto_sync(app: AppHandle, enabled: bool) -> Result<PublicSettings, String> {
    let mut settings = load_settings_file(&app)?;
    settings.auto_sync = enabled;
    write_settings_file(&app, &settings)?;
    Ok(public_settings(&settings))
}

#[tauri::command]
fn set_theme(app: AppHandle, theme: String) -> Result<PublicSettings, String> {
    if !["blue", "teal", "violet"].contains(&theme.as_str()) {
        return Err("不支持的主题颜色".to_string());
    }
    let mut settings = load_settings_file(&app)?;
    settings.theme = theme;
    write_settings_file(&app, &settings)?;
    Ok(public_settings(&settings))
}

#[tauri::command]
fn show_main_window(app: AppHandle, page: Option<String>) -> Result<(), String> {
    let window = app
        .get_webview_window("main")
        .ok_or_else(|| "主窗口不存在".to_string())?;
    if let Some(page) =
        page.filter(|value| ["conversations", "sync", "settings"].contains(&value.as_str()))
    {
        let _ = window.eval(&format!(
            "window.location.hash = {};",
            serde_json::to_string(&page).unwrap_or_else(|_| "\"conversations\"".to_string())
        ));
    }
    window.show().map_err(|error| error.to_string())?;
    window.unminimize().map_err(|error| error.to_string())?;
    window.set_focus().map_err(|error| error.to_string())
}

#[tauri::command]
fn hide_tray_window(app: AppHandle) -> Result<(), String> {
    if let Some(window) = app.get_webview_window("tray") {
        window.hide().map_err(|error| error.to_string())?;
    }
    Ok(())
}

#[tauri::command]
fn quit_app(app: AppHandle) {
    app.exit(0);
}

fn position_and_show_tray(app: &AppHandle, x: i32, y: i32) {
    if let Some(window) = app.get_webview_window("tray") {
        if let Ok(size) = window.outer_size() {
            let menu_x = (x - size.width as i32).max(8);
            let menu_y = (y - size.height as i32 - 8).max(8);
            let _ = window.set_position(Position::Physical(PhysicalPosition::new(menu_x, menu_y)));
        }
        let _ = window.show();
        let _ = window.set_focus();
    }
}

fn start_background_sync(app: &tauri::App) {
    let handle = app.handle().clone();
    let runtime = app.state::<AppRuntime>().inner().clone();
    tauri::async_runtime::spawn(async move {
        let mut interval = tokio::time::interval(Duration::from_secs(60));
        interval.tick().await;
        loop {
            interval.tick().await;
            let Ok(settings) = load_settings_file(&handle) else {
                continue;
            };
            if !settings.auto_sync || !configured(&settings) {
                continue;
            }
            let _ = perform_sync(&handle, &runtime, false).await;
        }
    });
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .manage(AppRuntime::default())
        .plugin(tauri_plugin_single_instance::init(|app, _, _| {
            let _ = show_main_window(app.clone(), None);
        }))
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_window_state::Builder::default().build())
        .plugin(tauri_plugin_autostart::init(
            MacosLauncher::LaunchAgent,
            Some(vec!["--background"]),
        ))
        .setup(|app| {
            let sync_shortcut =
                Shortcut::new(Some(Modifiers::CONTROL | Modifiers::ALT), Code::KeyP);
            let handler_shortcut = sync_shortcut.clone();
            app.handle().plugin(
                tauri_plugin_global_shortcut::Builder::new()
                    .with_handler(move |app, shortcut, event| {
                        if shortcut == &handler_shortcut && event.state() == ShortcutState::Pressed
                        {
                            let app = app.clone();
                            let runtime = app.state::<AppRuntime>().inner().clone();
                            tauri::async_runtime::spawn(async move {
                                let _ = perform_sync(&app, &runtime, true).await;
                            });
                        }
                    })
                    .build(),
            )?;
            app.global_shortcut().register(sync_shortcut)?;

            WebviewWindowBuilder::new(app, "tray", WebviewUrl::App("index.html?view=tray".into()))
                .title("Codex Continuity")
                .inner_size(500.0, 790.0)
                .decorations(false)
                .resizable(false)
                .always_on_top(true)
                .skip_taskbar(true)
                .transparent(true)
                .visible(false)
                .build()?;

            TrayIconBuilder::with_id("main-tray")
                .icon(app.default_window_icon().expect("app icon").clone())
                .tooltip("Codex Continuity · 自动同步运行中")
                .show_menu_on_left_click(false)
                .on_tray_icon_event(|tray, event| match event {
                    TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } => {
                        let _ = show_main_window(tray.app_handle().clone(), None);
                    }
                    TrayIconEvent::Click {
                        button: MouseButton::Right,
                        button_state: MouseButtonState::Up,
                        position,
                        ..
                    } => {
                        position_and_show_tray(
                            tray.app_handle(),
                            position.x as i32,
                            position.y as i32,
                        );
                    }
                    _ => {}
                })
                .build(app)?;
            start_background_sync(app);
            if std::env::args().any(|argument| argument == "--background") {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.hide();
                }
            }
            Ok(())
        })
        .on_window_event(|window, event| match event {
            WindowEvent::CloseRequested { api, .. } if window.label() == "main" => {
                api.prevent_close();
                let _ = window.hide();
            }
            WindowEvent::Focused(false) if window.label() == "tray" => {
                let _ = window.hide();
            }
            _ => {}
        })
        .invoke_handler(tauri::generate_handler![
            get_settings,
            save_settings,
            test_connection,
            test_upload,
            get_dashboard,
            sync_now,
            continue_conversation,
            export_archive,
            import_archive,
            set_auto_sync,
            set_theme,
            show_main_window,
            hide_tray_window,
            quit_app
        ])
        .run(tauri::generate_context!())
        .expect("error while running Codex Continuity");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalizes_server_urls() {
        assert_eq!(
            normalized_url(" https://continuity.example.com/ ").unwrap(),
            "https://continuity.example.com"
        );
        assert!(normalized_url("continuity.example.com").is_err());
    }

    #[test]
    fn creates_authenticated_upload_test_payload() {
        let key = [7u8; 32];
        let (plaintext_bytes, encrypted, digest) = encrypted_test_payload(&key).unwrap();
        assert_eq!(plaintext_bytes, 64 * 1024);
        assert_eq!(encrypted.len(), 64 * 1024 + 12 + 16);
        assert_eq!(digest, format!("{:x}", Sha256::digest(&encrypted)));
    }

    #[test]
    fn parses_server_timestamps() {
        assert!(parse_time("2026-07-27T10:20:30Z") > 0);
        assert_eq!(parse_time("invalid"), 0);
    }
}
