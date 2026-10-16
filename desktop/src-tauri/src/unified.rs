use super::*;

pub(super) fn secret_name(name: &str) -> String {
    if name == "encryption-key" || name == "key-fingerprint" {
        if let Some(session) = load_auth_session().filter(|s| s.unified) {
            return format!("{name}-{}", session.user.id);
        }
    }
    name.to_string()
}
pub(super) fn account_dir(app: &AppHandle) -> Result<PathBuf, String> {
    let root = app.path().app_config_dir().map_err(|e| e.to_string())?;
    let path = if let Some(session) = load_auth_session().filter(|s| s.unified) {
        root.join("accounts")
            .join(format!("{:x}", Sha256::digest(session.user.id.as_bytes())))
    } else {
        root
    };
    fs::create_dir_all(&path).map_err(|e| e.to_string())?;
    Ok(path)
}
#[tauri::command]
pub(super) async fn connect_account(
    app: AppHandle,
    runtime: State<'_, AppRuntime>,
) -> Result<AuthActionResult, String> {
    let _guard = runtime.sync_lock.lock().await;
    let grant = tauri::async_runtime::spawn_blocking(native_login::browser_login)
        .await
        .map_err(|e| e.to_string())?
        .map_err(|e| e.to_string())?;
    let mut candidate = [0u8; 32];
    rand::rng().fill_bytes(&mut candidate);
    let candidate_hash = format!("{:x}", Sha256::digest(candidate));
    // Persist before requesting first-use initialization. A process interruption
    // after the server commit must not lose the only copy of a new account key.
    write_secret(
        &format!("pending-unified-key-{candidate_hash}"),
        &STANDARD_NO_PAD.encode(candidate),
    )?;
    let mut input = serde_json::to_value(grant).map_err(|e| e.to_string())?;
    input["keyHash"] = json!(format!("{:x}", Sha256::digest(candidate)));
    let response = reqwest::Client::builder()
        .timeout(Duration::from_secs(15))
        .redirect(reqwest::redirect::Policy::none())
        .build()
        .map_err(|e| e.to_string())?
        .post(format!("{DEFAULT_SERVER_URL}/api/v1/client/auth/fribench"))
        .json(&input)
        .send()
        .await
        .map_err(|e| e.to_string())?;
    let envelope = parse_auth_response(response).await?;
    let hash = envelope.wrapped_key.clone();
    let session = stored_session_from_envelope(&envelope);
    let mut settings = load_settings_file(&app)?;
    settings.device_id = register_current_device(
        &settings,
        DEFAULT_SERVER_URL,
        &envelope.access_token,
        &envelope.user.id,
    )
    .await?;
    write_auth_session(&session)?;
    let recovered_pending = read_secret(&format!("pending-unified-key-{hash}"));
    if let Some(key) = recovered_pending.as_ref() {
        write_secret("encryption-key", key)?;
        delete_secret(&format!("pending-unified-key-{hash}"));
    }
    if candidate_hash != hash {
        delete_secret(&format!("pending-unified-key-{candidate_hash}"));
    }
    if let Some(key) = read_secret("encryption-key") {
        if STANDARD_NO_PAD
            .decode(&key)
            .ok()
            .map(|k| format!("{:x}", Sha256::digest(k)))
            .as_ref()
            != Some(&hash)
        {
            delete_secret("encryption-key");
        }
    }
    settings.server_url = DEFAULT_SERVER_URL.to_string();
    settings.auto_sync = false;
    write_settings_file(&app, &settings)?;
    write_secret("key-fingerprint", &hash)?;
    let status = auth_status_for(&settings);
    Ok(AuthActionResult {
        message: if status.encryption_unlocked {
            "账号已连接，本机密钥自动解锁；首次不会上传旧会话"
        } else {
            "账号已连接，请输入离线保存的恢复密钥解锁历史快照"
        }
        .to_string(),
        status,
        recovery_key: recovered_pending,
    })
}
#[tauri::command]
pub(super) fn unlock_account(
    app: AppHandle,
    recovery_key: String,
) -> Result<AuthActionResult, String> {
    let _session = load_auth_session()
        .filter(|s| s.unified)
        .ok_or("请先连接统一账号")?;
    let key = decode_recovery_key(
        &recovery_key,
        &read_secret("key-fingerprint").ok_or("密钥指纹未就绪，请重新连接账号")?,
    )?;
    write_secret("encryption-key", &STANDARD_NO_PAD.encode(key))?;
    Ok(AuthActionResult {
        status: auth_status_for(&load_settings_file(&app)?),
        message: "本机加密密钥已解锁".into(),
        recovery_key: None,
    })
}
fn decode_recovery_key(value: &str, fingerprint: &str) -> Result<Vec<u8>, String> {
    let key = STANDARD_NO_PAD
        .decode(value.trim())
        .map_err(|_| "恢复密钥格式无效")?;
    if key.len() != 32 || format!("{:x}", Sha256::digest(&key)) != fingerprint {
        return Err("恢复密钥不匹配；本地和云端数据未被修改".into());
    }
    Ok(key)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn recovery_key_is_verified_locally_and_rejects_wrong_account() {
        let key = [4u8; 32];
        let value = STANDARD_NO_PAD.encode(key);
        let hash = format!("{:x}", Sha256::digest(key));
        assert_eq!(decode_recovery_key(&value, &hash).unwrap(), key);
        assert!(decode_recovery_key(&value, &"a".repeat(64)).is_err());
        assert!(decode_recovery_key("invalid", &hash).is_err());
    }
}
#[tauri::command]
pub(super) async fn guest_archive(
    app: AppHandle,
    runtime: State<'_, AppRuntime>,
    confirm: bool,
) -> Result<String, String> {
    let _guard = runtime.sync_lock.lock().await;
    let settings = load_settings_file(&app)?;
    ensure_access_token(&settings).await?;
    let config = temp_core_config(&settings)?;
    let folder = account_dir(&app)?.join("guest-preview");
    if confirm {
        if !folder.join("preview.json").is_file() {
            return Err("请先预览游客存档".into());
        }
        return run_core_output(
            &app,
            vec![
                "guest-confirm".into(),
                "--config".into(),
                config.path().to_string_lossy().into(),
                "--folder".into(),
                folder.to_string_lossy().into(),
            ],
        )
        .await;
    }
    if folder.exists() {
        return Err("已有待确认预览；可以确认或忽略，30 分钟后过期，不占用绑定机会".into());
    }
    let guest = match read_secret("guest-installation") {
        Some(v) => v,
        None => {
            let value =
                base64::engine::general_purpose::URL_SAFE_NO_PAD.encode(rand::random::<[u8; 32]>());
            write_secret("guest-installation", &value)?;
            value
        }
    };
    run_core_output(
        &app,
        vec![
            "guest-prepare".into(),
            "--config".into(),
            config.path().to_string_lossy().into(),
            "--folder".into(),
            folder.to_string_lossy().into(),
            "--guest".into(),
            guest,
        ],
    )
    .await
}
#[tauri::command]
pub(super) async fn cancel_guest_preview(
    app: AppHandle,
    runtime: State<'_, AppRuntime>,
) -> Result<(), String> {
    let _guard = runtime.sync_lock.lock().await;
    let folder = account_dir(&app)?.join("guest-preview");
    for name in ["preview.json", "snapshot.ccx"] {
        let path = folder.join(name);
        if path.is_file() {
            fs::remove_file(path).map_err(|e| e.to_string())?
        }
    }
    if folder.is_dir() {
        fs::remove_dir(folder).map_err(|e| e.to_string())?
    }
    Ok(())
}
