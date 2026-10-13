// No console window on Windows release builds.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod commands;
mod jingle;
mod sync;
mod tray;

use tauri::{AppHandle, Manager, WindowEvent};

/// Shows and focuses the settings window.
pub fn show_settings(app: &AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.show();
        let _ = w.unminimize();
        let _ = w.set_focus();
    }
}

fn main() {
    tauri::Builder::default()
        // A second launch opens the running app's window instead of a second tray icon.
        .plugin(tauri_plugin_single_instance::init(|app, _, _| show_settings(app)))
        .plugin(tauri_plugin_dialog::init())
        .manage(sync::AppState::default())
        .invoke_handler(tauri::generate_handler![
            commands::snapshot,
            commands::relocate,
            commands::schema,
            commands::version,
            commands::config_set,
            commands::config_add,
            commands::config_remove,
            commands::play,
            commands::preview,
            commands::install,
            commands::uninstall,
            commands::doctor_repo,
        ])
        .setup(|app| {
            // A menu-bar app: no Dock icon.
            #[cfg(target_os = "macos")]
            app.set_activation_policy(tauri::ActivationPolicy::Accessory);
            let items = tray::build(app.handle())?;
            let _ = app.state::<sync::AppState>().tray.set(items);
            sync::spawn_watcher(app.handle().clone());
            // `--hidden` starts in the menu bar only (for launch at login).
            if !std::env::args().any(|a| a == "--hidden") {
                show_settings(app.handle());
            }
            Ok(())
        })
        .on_window_event(|window, event| match event {
            // Closing the window keeps the app in the menu bar.
            WindowEvent::CloseRequested { api, .. } => {
                api.prevent_close();
                let _ = window.hide();
            }
            WindowEvent::Focused(true) => sync::refresh_soon(window.app_handle()),
            _ => {}
        })
        .run(tauri::generate_context!())
        .expect("error while running jingle");
}
