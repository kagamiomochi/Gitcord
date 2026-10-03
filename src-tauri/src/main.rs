#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]
use std::net::{TcpListener, TcpStream};
use std::sync::Mutex;
use std::time::{Duration, Instant};
use tauri::{Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_shell::{process::CommandChild, ShellExt};

struct Server(Mutex<Option<CommandChild>>);

fn main() {
    // Wayland + NVIDIA で WebKitGTK が "Error 71 (Protocol error)" で落ちる問題の回避
    #[cfg(target_os = "linux")]
    if std::env::var_os("WEBKIT_DISABLE_DMABUF_RENDERER").is_none() {
        std::env::set_var("WEBKIT_DISABLE_DMABUF_RENDERER", "1");
    }

    let app = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            // 空きポートを選び、Goバックエンド(sidecar)を起動する
            let port = TcpListener::bind("127.0.0.1:0")?.local_addr()?.port();
            let (mut rx, child) = app
                .shell()
                .sidecar("gitcord-server")?
                .args(["--addr", &format!("127.0.0.1:{port}"), "--no-open"])
                .spawn()?;
            tauri::async_runtime::spawn(async move { while rx.recv().await.is_some() {} });
            app.manage(Server(Mutex::new(Some(child))));

            let deadline = Instant::now() + Duration::from_secs(10);
            while TcpStream::connect(("127.0.0.1", port)).is_err() && Instant::now() < deadline {
                std::thread::sleep(Duration::from_millis(50));
            }
            WebviewWindowBuilder::new(
                app,
                "main",
                WebviewUrl::External(format!("http://127.0.0.1:{port}").parse()?),
            )
            .title("Gitcord")
            .inner_size(1280.0, 800.0)
            .build()?;
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("failed to build Gitcord");

    app.run(|handle, event| {
        if let RunEvent::Exit = event {
            if let Some(c) = handle.state::<Server>().0.lock().unwrap().take() {
                let _ = c.kill();
            }
        }
    });
}
