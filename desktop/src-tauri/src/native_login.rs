//! Native public-client login. No app secret or user password leaves the browser.
use anyhow::{Context, Result};
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use serde::Serialize;
use sha2::{Digest, Sha256};
use std::{
    io::{Read, Write},
    net::TcpListener,
    time::{Duration, Instant},
};

#[derive(Serialize)]
pub struct Grant {
    pub code: String,
    pub code_verifier: String,
    pub redirect_uri: String,
}
pub fn browser_login() -> Result<Grant> {
    let listener = TcpListener::bind("127.0.0.1:0")?;
    listener.set_nonblocking(true)?;
    let port = listener.local_addr()?.port();
    let redirect_uri = format!("http://127.0.0.1:{port}/fribench/callback");
    let verifier = URL_SAFE_NO_PAD.encode(rand::random::<[u8; 32]>());
    let state = URL_SAFE_NO_PAD.encode(rand::random::<[u8; 32]>());
    let challenge = URL_SAFE_NO_PAD.encode(Sha256::digest(verifier.as_bytes()));
    let mut url = reqwest::Url::parse("https://continuity.fribench.cn/desktop-connect")?;
    url.query_pairs_mut()
        .append_pair("code_challenge", &challenge)
        .append_pair("state", &state)
        .append_pair("redirect_uri", &redirect_uri);
    open_browser(url.as_str())?;
    let deadline = Instant::now() + Duration::from_secs(300);
    while Instant::now() < deadline {
        match listener.accept() {
            Ok((mut stream, peer)) => {
                if !peer.ip().is_loopback() {
                    continue;
                }
                stream.set_read_timeout(Some(Duration::from_secs(2)))?;
                stream.set_write_timeout(Some(Duration::from_secs(2)))?;
                let mut bytes = [0u8; 8192];
                let size = match stream.read(&mut bytes) {
                    Ok(size) => size,
                    Err(_) => continue,
                };
                let raw = String::from_utf8_lossy(&bytes[..size]);
                let Some(path) = raw
                    .lines()
                    .next()
                    .and_then(|line| line.strip_prefix("GET "))
                    .and_then(|line| line.split(' ').next())
                else {
                    continue;
                };
                let uri = match reqwest::Url::parse(&format!("http://127.0.0.1:{port}{path}")) {
                    Ok(uri) => uri,
                    Err(_) => continue,
                };
                let pairs: std::collections::HashMap<_, _> =
                    uri.query_pairs().into_owned().collect();
                if uri.path() != "/fribench/callback" || pairs.get("state") != Some(&state) {
                    let _=stream.write_all(b"HTTP/1.1 400 Bad Request\r\nConnection: close\r\nContent-Length: 0\r\n\r\n");
                    continue;
                }
                let code = pairs.get("code").context("登录响应缺少授权码")?.to_owned();
                anyhow::ensure!(
                    code.len() == 43
                        && code
                            .bytes()
                            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-'),
                    "登录授权码无效"
                );
                let page = "<!doctype html><meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'><title>Fribench 已连接</title><h1>浏览器验证已完成</h1><p>请回到桌面应用继续。此页面可以关闭。</p>";
                let _ = write!(
                    stream,
                    "HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nCache-Control: no-store\r\nReferrer-Policy: no-referrer\r\nConnection: close\r\nContent-Length: {}\r\n\r\n{}",
                    page.len(),
                    page
                );
                return Ok(Grant {
                    code,
                    code_verifier: verifier,
                    redirect_uri,
                });
            }
            Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                std::thread::sleep(Duration::from_millis(50))
            }
            Err(error) => return Err(error.into()),
        }
    }
    anyhow::bail!("浏览器登录等待已过期，请重新发起")
}
fn open_browser(url: &str) -> Result<()> {
    #[cfg(target_os = "windows")]
    {
        use std::os::windows::process::CommandExt;
        std::process::Command::new("rundll32.exe")
            .args(["url.dll,FileProtocolHandler", url])
            .creation_flags(0x08000000)
            .spawn()?;
    }
    #[cfg(not(target_os = "windows"))]
    {
        std::process::Command::new("xdg-open").arg(url).spawn()?;
    }
    Ok(())
}
