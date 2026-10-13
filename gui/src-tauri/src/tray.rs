//! The menu-bar / tray icon and its menu. Each item runs one jingle command;
//! the check marks always come from the last `jingle status`.

use serde_json::Value;
use tauri::image::Image;
use tauri::menu::{CheckMenuItem, Menu, MenuEvent, MenuItem, PredefinedMenuItem, Submenu};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Wry};

use crate::sync::{self, Snapshot};

/// The events jingle knows (`events` in `jingle schema`), with menu labels.
const EVENTS: [(&str, &str); 3] = [("commit", "Commit"), ("push", "Push"), ("merge", "Merge")];

pub struct TrayItems {
    status: MenuItem<Wry>,
    enabled: CheckMenuItem<Wry>,
    events: Vec<(&'static str, CheckMenuItem<Wry>)>,
    play: Submenu<Wry>,
}

pub fn build(app: &AppHandle) -> tauri::Result<TrayItems> {
    let status = MenuItem::with_id(app, "status", "jingle", true, None::<&str>)?;
    let enabled = CheckMenuItem::with_id(app, "enabled", "Sounds on", true, false, None::<&str>)?;
    let mut events = Vec::new();
    for (ev, label) in EVENTS {
        events.push((ev, CheckMenuItem::with_id(app, format!("event:{ev}"), label, true, false, None::<&str>)?));
    }
    let play = Submenu::with_id(app, "play", "Play", true)?;
    for (ev, label) in EVENTS {
        play.append(&MenuItem::with_id(app, format!("play:{ev}"), label, true, None::<&str>)?)?;
    }
    let menu = Menu::new(app)?;
    menu.append(&status)?;
    menu.append(&PredefinedMenuItem::separator(app)?)?;
    menu.append(&enabled)?;
    for (_, item) in &events {
        menu.append(item)?;
    }
    menu.append(&PredefinedMenuItem::separator(app)?)?;
    menu.append(&play)?;
    menu.append(&MenuItem::with_id(app, "settings", "Settings…", true, Some("CmdOrCtrl+,"))?)?;
    menu.append(&PredefinedMenuItem::separator(app)?)?;
    menu.append(&MenuItem::with_id(app, "quit", "Quit Jingle", true, Some("CmdOrCtrl+Q"))?)?;

    TrayIconBuilder::with_id("jingle")
        .icon(Image::from_bytes(include_bytes!("../icons/tray.png"))?)
        .icon_as_template(true)
        .tooltip("Jingle")
        .menu(&menu)
        .show_menu_on_left_click(true)
        .on_menu_event(on_menu_event)
        .on_tray_icon_event(|tray, _| sync::refresh_soon(tray.app_handle()))
        .build(app)?;
    Ok(TrayItems { status, enabled, events, play })
}

fn on_menu_event(app: &AppHandle, event: MenuEvent) {
    let id = event.id().as_ref().to_string();
    match id.as_str() {
        "settings" | "status" => crate::show_settings(app),
        "quit" => app.exit(0),
        "enabled" => toggle(app, "enabled"),
        _ => {
            if let Some(ev) = id.strip_prefix("event:") {
                toggle(app, &format!("{ev}.enabled"));
            } else if let Some(ev) = id.strip_prefix("play:") {
                let (app, ev) = (app.clone(), ev.to_string());
                std::thread::spawn(move || {
                    if let Ok(j) = sync::current(&app) {
                        let _ = j.run(&["play"], &[&ev]);
                    }
                });
            }
        }
    }
}

/// Flips a bool config key, from its value in the last status (the menu's
/// own check mark may already have flipped), then refreshes.
fn toggle(app: &AppHandle, key: &str) {
    let current = sync::last(app).status.as_ref().and_then(|s| config_bool(s, key));
    let (app, key) = (app.clone(), key.to_string());
    std::thread::spawn(move || {
        if let (Some(current), Ok(j)) = (current, sync::current(&app)) {
            let _ = j.run(&["config", "set"], &[&key, if current { "false" } else { "true" }]);
        }
        sync::refresh(&app);
    });
}

fn config_bool(status: &Value, key: &str) -> Option<bool> {
    status.get("config")?.get(key)?.as_bool()
}

impl TrayItems {
    pub fn update(&self, snap: &Snapshot) {
        let status = snap.status.as_ref();
        let _ = self.status.set_text(&snap.headline);
        let ok = status.is_some();
        let _ = self.enabled.set_enabled(ok);
        let _ = self.enabled.set_checked(status.and_then(|s| config_bool(s, "enabled")).unwrap_or(false));
        for (ev, item) in &self.events {
            let _ = item.set_enabled(ok);
            let _ = item.set_checked(status.and_then(|s| config_bool(s, &format!("{ev}.enabled"))).unwrap_or(false));
        }
        let _ = self.play.set_enabled(ok);
    }
}

/// One line saying whether sounds will play, shown at the top of the menu.
pub fn headline(snap: &Snapshot) -> String {
    let Some(status) = snap.status.as_ref() else {
        return match &snap.error {
            Some(e) if e.code == crate::jingle::CODE_NOT_FOUND => "jingle CLI not found".into(),
            Some(e) if e.code == crate::jingle::CODE_CLI_OLD => "jingle CLI needs an update".into(),
            Some(e) if e.code == crate::jingle::CODE_APP_OLD => "This app needs an update".into(),
            _ => "jingle isn't responding".into(),
        };
    };
    let errors = status
        .get("problems")
        .and_then(Value::as_array)
        .map(|ps| ps.iter().filter(|p| p.get("severity").and_then(Value::as_str) == Some("error")).count())
        .unwrap_or(0);
    if status.get("installed").and_then(Value::as_bool) != Some(true) {
        "Not installed".into()
    } else if config_bool(status, "enabled") == Some(false) {
        "Off".into()
    } else if errors > 0 {
        format!("On, {errors} problem{}", if errors == 1 { "" } else { "s" })
    } else {
        "On".into()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::jingle::JingleError;
    use serde_json::json;

    fn snap(status: Value) -> Snapshot {
        Snapshot { status: Some(status), ..Default::default() }
    }

    #[test]
    fn headline_says_why_nothing_plays() {
        let none = Snapshot { error: Some(JingleError::new(crate::jingle::CODE_NOT_FOUND, "x")), ..Default::default() };
        assert_eq!(headline(&none), "jingle CLI not found");
        assert_eq!(headline(&snap(json!({"installed": false, "config": {"enabled": true}}))), "Not installed");
        assert_eq!(headline(&snap(json!({"installed": true, "config": {"enabled": false}}))), "Off");
        let problems = json!([{"severity": "error"}, {"severity": "warning"}]);
        assert_eq!(
            headline(&snap(json!({"installed": true, "config": {"enabled": true}, "problems": problems}))),
            "On, 1 problem"
        );
        assert_eq!(headline(&snap(json!({"installed": true, "config": {"enabled": true}, "problems": []}))), "On");
    }
}
