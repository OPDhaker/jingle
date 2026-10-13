//! Keeps the GUI in step with jingle. The window and tray render from one
//! `Snapshot` (the output of `jingle status`); it is refreshed after every
//! change the GUI makes, when the window gains focus, and whenever a file
//! jingle's status depends on changes, so a `jingle config set` typed in a
//! terminal shows up within a couple of seconds.

use std::env;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};
use std::thread;
use std::time::{Duration, SystemTime};

use serde::Serialize;
use serde_json::Value;
use tauri::{AppHandle, Emitter, Manager};

use crate::jingle::{self, Jingle, JingleError};
use crate::tray::TrayItems;

const POLL: Duration = Duration::from_millis(1500);
/// While jingle isn't found (or fails), try again every this many polls,
/// so installing the CLI with the app open just works.
const RELOCATE_EVERY: u32 = 4;

/// What the window and tray show.
#[derive(Debug, Clone, Default, Serialize)]
pub struct Snapshot {
    /// The jingle binary in use; null when it wasn't found.
    pub jingle: Option<Jingle>,
    /// `jingle status --json`; null when it couldn't be read.
    pub status: Option<Value>,
    /// Why there is no status.
    pub error: Option<JingleError>,
    /// One line saying whether sounds will play (also the tray's top line).
    pub headline: String,
}

#[derive(Default)]
pub struct AppState {
    jingle: Mutex<Option<Jingle>>,
    last: Mutex<Snapshot>,
    pub tray: OnceLock<TrayItems>,
}

/// The located jingle, locating it on first use.
pub fn current(app: &AppHandle) -> Result<Jingle, JingleError> {
    let state = app.state::<AppState>();
    let mut j = state.jingle.lock().unwrap();
    if j.is_none() {
        *j = Some(jingle::locate()?);
    }
    Ok(j.clone().unwrap())
}

/// Forgets the located binary, so the next call looks again.
pub fn forget(app: &AppHandle) {
    *app.state::<AppState>().jingle.lock().unwrap() = None;
}

/// The last snapshot, without running jingle.
pub fn last(app: &AppHandle) -> Snapshot {
    app.state::<AppState>().last.lock().unwrap().clone()
}

/// Runs `jingle status`, then updates the tray and tells the window.
pub fn refresh(app: &AppHandle) -> Snapshot {
    let mut snap = match current(app) {
        Err(e) => Snapshot { error: Some(e), ..Default::default() },
        Ok(j) => match j.check_compatible().and_then(|()| j.run(&["status"], &[])) {
            Ok(status) => Snapshot { jingle: Some(j), status: Some(status), ..Default::default() },
            Err(e) => Snapshot { jingle: Some(j), error: Some(e), ..Default::default() },
        },
    };
    snap.headline = crate::tray::headline(&snap);
    #[cfg(debug_assertions)]
    eprintln!("jingle-gui: refreshed: {}", snap.headline);
    let state = app.state::<AppState>();
    *state.last.lock().unwrap() = snap.clone();
    if let Some(items) = state.tray.get() {
        items.update(&snap);
    }
    let _ = app.emit("state-changed", &snap);
    snap
}

/// Same as refresh, off the calling thread.
pub fn refresh_soon(app: &AppHandle) {
    let app = app.clone();
    thread::spawn(move || {
        refresh(&app);
    });
}

pub fn spawn_watcher(app: AppHandle) {
    thread::spawn(move || {
        let mut seen = watched(&refresh(&app)).map(|w| fingerprint(&w));
        let mut tick = 0u32;
        loop {
            thread::sleep(POLL);
            tick += 1;
            let snap = last(&app);
            let now = match watched(&snap) {
                Some(w) => Some(fingerprint(&w)),
                // jingle missing or failing: try again now and then.
                None if tick.is_multiple_of(RELOCATE_EVERY) => {
                    if snap.jingle.is_none() {
                        forget(&app);
                    }
                    None
                }
                None => continue,
            };
            if now.is_none() || now != seen {
                seen = watched(&refresh(&app)).map(|w| fingerprint(&w));
            }
        }
    });
}

/// Files whose change can change `jingle status`: the config, jingle's
/// install state, the global gitconfig (core.hooksPath), and the binary.
fn watched(snap: &Snapshot) -> Option<Vec<PathBuf>> {
    let status = snap.status.as_ref()?;
    let mut files = vec![snap.jingle.as_ref()?.bin.clone()];
    if let Some(p) = status.get("config_path").and_then(Value::as_str) {
        files.push(p.into());
    }
    if let Some(dir) = status.get("hooks_dir").and_then(Value::as_str).and_then(|d| Path::new(d).parent()) {
        files.push(dir.join("state.json"));
    }
    files.extend(gitconfig_files());
    Some(files)
}

fn gitconfig_files() -> Vec<PathBuf> {
    if let Some(p) = env::var_os("GIT_CONFIG_GLOBAL").filter(|p| !p.is_empty()) {
        return vec![p.into()];
    }
    let Some(home) = env::var_os("HOME").map(PathBuf::from) else { return vec![] };
    let xdg = env::var_os("XDG_CONFIG_HOME").map(PathBuf::from).unwrap_or_else(|| home.join(".config"));
    vec![home.join(".gitconfig"), xdg.join("git/config")]
}

type Stamp = Option<(SystemTime, u64)>;

fn fingerprint(files: &[PathBuf]) -> Vec<Stamp> {
    files.iter().map(|f| f.metadata().ok().and_then(|m| Some((m.modified().ok()?, m.len())))).collect()
}
