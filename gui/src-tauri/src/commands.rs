//! What the settings window may ask for. Each command maps to one fixed
//! jingle invocation; the window can't run arbitrary arguments (so, for
//! example, no `uninstall --purge`). Commands that change something refresh
//! the shared snapshot afterwards, which re-renders the window and tray.

use serde_json::Value;
use tauri::AppHandle;

use crate::jingle::{JingleError, CODE_SPAWN};
use crate::sync::{self, Snapshot};

async fn call(app: AppHandle, cmd: Vec<String>, positional: Vec<String>, changes: bool) -> Result<Value, JingleError> {
    tauri::async_runtime::spawn_blocking(move || {
        let j = sync::current(&app)?;
        let cmd: Vec<&str> = cmd.iter().map(String::as_str).collect();
        let positional: Vec<&str> = positional.iter().map(String::as_str).collect();
        let res = j.run(&cmd, &positional);
        if changes {
            sync::refresh(&app);
        }
        res
    })
    .await
    .map_err(|e| JingleError::new(CODE_SPAWN, e.to_string()))?
}

fn strings<const N: usize>(a: [&str; N]) -> Vec<String> {
    a.iter().map(|s| s.to_string()).collect()
}

/// A fresh `jingle status`, as the window renders it.
#[tauri::command]
pub async fn snapshot(app: AppHandle) -> Snapshot {
    tauri::async_runtime::spawn_blocking(move || sync::refresh(&app)).await.unwrap_or_default()
}

/// Looks for the jingle binary again (after the user installs it).
#[tauri::command]
pub async fn relocate(app: AppHandle) -> Snapshot {
    sync::forget(&app);
    snapshot(app).await
}

#[tauri::command]
pub async fn schema(app: AppHandle) -> Result<Value, JingleError> {
    call(app, strings(["schema"]), vec![], false).await
}

#[tauri::command]
pub async fn version(app: AppHandle) -> Result<Value, JingleError> {
    call(app, strings(["version"]), vec![], false).await
}

/// `jingle config set <key> <values>...`
#[tauri::command]
pub async fn config_set(app: AppHandle, key: String, values: Vec<String>) -> Result<Value, JingleError> {
    call(app, strings(["config", "set"]), [vec![key], values].concat(), true).await
}

/// `jingle config add <event>.sounds <files>...`
#[tauri::command]
pub async fn config_add(app: AppHandle, key: String, sounds: Vec<String>) -> Result<Value, JingleError> {
    call(app, strings(["config", "add"]), [vec![key], sounds].concat(), true).await
}

/// `jingle config remove <event>.sounds <entries>...`
#[tauri::command]
pub async fn config_remove(app: AppHandle, key: String, sounds: Vec<String>) -> Result<Value, JingleError> {
    call(app, strings(["config", "remove"]), [vec![key], sounds].concat(), true).await
}

/// `jingle play <event> [--sound <entry>]`
#[tauri::command]
pub async fn play(app: AppHandle, event: String, sound: Option<String>) -> Result<Value, JingleError> {
    let mut cmd = strings(["play"]);
    if let Some(s) = sound {
        cmd.push(format!("--sound={s}"));
    }
    call(app, cmd, vec![event], false).await
}

/// `jingle preview <builtin:name|file>`
#[tauri::command]
pub async fn preview(app: AppHandle, sound: String) -> Result<Value, JingleError> {
    call(app, strings(["preview"]), vec![sound], false).await
}

#[tauri::command]
pub async fn install(app: AppHandle) -> Result<Value, JingleError> {
    call(app, strings(["install", "--yes"]), vec![], true).await
}

#[tauri::command]
pub async fn uninstall(app: AppHandle) -> Result<Value, JingleError> {
    call(app, strings(["uninstall", "--yes"]), vec![], true).await
}

/// `jingle doctor --repo <dir>`: checks one repo, e.g. for husky.
#[tauri::command]
pub async fn doctor_repo(app: AppHandle, dir: String) -> Result<Value, JingleError> {
    call(app, vec!["doctor".into(), format!("--repo={dir}")], vec![], false).await
}
