use std::env;
use std::path::PathBuf;

use noraneko_winupdater::config;
use noraneko_winupdater::updater::{Options, Updater};

const VERSION: &str = "1.0.0";

fn main() {
    let mut scheduled = false;
    let mut portable = false;
    let mut create_task = false;
    let mut remove_task = false;
    let mut check_only = false;
    let mut version = false;

    for arg in env::args().skip(1) {
        match arg.as_str() {
            "-scheduled" => scheduled = true,
            "-portable" => portable = true,
            "-create-task" => create_task = true,
            "-remove-task" => remove_task = true,
            "-check-only" => check_only = true,
            "-version" => version = true,
            _ => {}
        }
    }

    if version {
        println!("{} WinUpdater v{}", config::BROWSER_NAME, VERSION);
        return;
    }

    let exe_path = match env::current_exe() {
        Ok(path) => path,
        Err(err) => {
            eprintln!("Error getting executable path: {err}");
            std::process::exit(1);
        }
    };
    let exe_dir = exe_path
        .parent()
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("."));

    let cfg = match config::Config::load(&exe_dir) {
        Ok(cfg) => cfg,
        Err(err) => {
            eprintln!("Error loading configuration: {err}");
            std::process::exit(1);
        }
    };

    let mut updater = Updater::new(
        cfg,
        Options {
            scheduled,
            portable,
            check_only,
            create_task,
            remove_task,
            version: VERSION.to_string(),
        },
    );

    if create_task || remove_task {
        if let Err(err) = updater.handle_scheduled_task() {
            eprintln!("Error handling scheduled task: {err}");
            std::process::exit(1);
        }
        return;
    }

    if let Err(err) = updater.run() {
        eprintln!("Error: {err}");
        std::process::exit(1);
    }
}
