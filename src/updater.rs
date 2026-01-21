use crate::config;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::fs::{self, File};
use std::io::{self, Read};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::Duration;
use time::OffsetDateTime;
use time::macros::format_description;
use walkdir::WalkDir;
use zip::ZipArchive;

#[derive(Clone, Debug)]
pub struct Options {
    pub scheduled: bool,
    pub portable: bool,
    pub check_only: bool,
    pub create_task: bool,
    pub remove_task: bool,
    pub version: String,
}

pub struct Updater {
    cfg: config::Config,
    opts: Options,
    release: Option<Release>,
}

struct TempFileCleanup {
    path: PathBuf,
}

impl Drop for TempFileCleanup {
    fn drop(&mut self) {
        let _ = fs::remove_file(&self.path);
    }
}

#[derive(Debug, Deserialize)]
struct Release {
    #[serde(rename = "tag_name")]
    tag_name: String,
    assets: Vec<Asset>,
}

#[derive(Debug, Deserialize, Clone)]
struct Asset {
    name: String,
    #[serde(rename = "browser_download_url")]
    browser_download_url: String,
}

impl Updater {
    pub fn new(cfg: config::Config, opts: Options) -> Self {
        Self {
            cfg,
            opts,
            release: None,
        }
    }

    pub fn run(&mut self) -> Result<(), Box<dyn std::error::Error>> {
        println!("Noraneko WinUpdater v{}", self.opts.version);
        println!("Checking for updates...");

        self.check_connection()?;

        let current_version = match self.get_current_version() {
            Ok(version) => version,
            Err(err) => {
                println!("Could not determine current version: {err}");
                "0.0.0".to_string()
            }
        };
        println!("Current version: {current_version}");

        let release = self.get_latest_release()?;
        let new_version = release.tag_name.trim_start_matches('v').to_string();
        println!("Latest version: {new_version}");
        self.release = Some(release);

        if !Self::is_newer_version(&current_version, &new_version) {
            println!("No new version available.");
            self.log_result("No new version found");
            return Ok(());
        }

        println!("New version available: {current_version} -> {new_version}");
        if self.opts.check_only {
            println!("Check-only mode, not installing.");
            return Ok(());
        }

        self.download_and_install()?;
        println!("Update completed successfully!");
        self.log_result(&format!("Updated from {current_version} to {new_version}"));
        Ok(())
    }

    pub fn handle_scheduled_task(&self) -> Result<(), Box<dyn std::error::Error>> {
        let script_name = if self.opts.create_task {
            "ScheduledTask-Create.ps1"
        } else if self.opts.remove_task {
            "ScheduledTask-Remove.ps1"
        } else {
            return Ok(());
        };

        let script_path = self.cfg.exe_dir.join(script_name);
        if !script_path.exists() {
            return Err(format!("scheduled task script not found: {}", script_path.display()).into());
        }

        let status = Command::new("powershell.exe")
            .args([
                "-NoProfile",
                "-ExecutionPolicy",
                "RemoteSigned",
                "-File",
                script_path.to_string_lossy().as_ref(),
            ])
            .status()?;

        if status.success() {
            Ok(())
        } else {
            Err("scheduled task script failed".into())
        }
    }

    fn check_connection(&self) -> Result<(), Box<dyn std::error::Error>> {
        let response = ureq::get(config::CONNECT_CHECK_URL)
            .timeout(Duration::from_secs(30))
            .call();
        let response = match response {
            Ok(response) => response,
            Err(ureq::Error::Status(code, _)) => {
                return Err(format!("API returned status {}", code).into());
            }
            Err(err) => return Err(err.into()),
        };
        if response.status() >= 400 {
            return Err(format!("API returned status {}", response.status()).into());
        }
        Ok(())
    }

    fn get_current_version(&self) -> Result<String, Box<dyn std::error::Error>> {
        let browser_path = self.cfg.get_browser_path();
        if browser_path.is_empty() {
            return Err("browser not found".into());
        }

        let browser_dir = Path::new(&browser_path)
            .parent()
            .ok_or_else(|| "invalid browser path")?;

        let app_ini_path = browser_dir.join("application.ini");
        if let Ok(data) = fs::read_to_string(&app_ini_path) {
            for line in data.lines() {
                if let Some(rest) = line.strip_prefix("Version=") {
                    return Ok(rest.trim().to_string());
                }
            }
        }

        let version_path = browser_dir.join("version");
        if let Ok(data) = fs::read_to_string(&version_path) {
            return Ok(data.trim().to_string());
        }

        Err("could not determine version".into())
    }

    fn get_latest_release(&self) -> Result<Release, Box<dyn std::error::Error>> {
        let url = format!("{}/latest", config::RELEASE_API_URL);
        let response = ureq::get(&url)
            .set("Accept", "application/vnd.github.v3+json")
            .set("User-Agent", &format!("Noraneko-WinUpdater/{}", self.opts.version))
            .timeout(Duration::from_secs(300))
            .call();
        let response = match response {
            Ok(response) => response,
            Err(ureq::Error::Status(code, response)) => {
                let body = response.into_string().unwrap_or_default();
                return Err(format!("API returned status {}: {}", code, body).into());
            }
            Err(err) => return Err(err.into()),
        };
        let status = response.status();
        let body = response.into_string().unwrap_or_default();
        if status != 200 {
            return Err(format!("API returned status {}: {}", status, body).into());
        }
        let release: Release = serde_json::from_str(&body)?;
        Ok(release)
    }

    fn is_newer_version(current: &str, latest: &str) -> bool {
        let current = current.trim_start_matches('v');
        let latest = latest.trim_start_matches('v');

        if current.is_empty() || current == "0.0.0" {
            return true;
        }
        if latest == current {
            return false;
        }

        let current_parts = Self::parse_version(current);
        let latest_parts = Self::parse_version(latest);
        let max_len = current_parts.len().max(latest_parts.len());
        for i in 0..max_len {
            let cp = current_parts.get(i).cloned().unwrap_or(0);
            let lp = latest_parts.get(i).cloned().unwrap_or(0);
            if lp > cp {
                return true;
            }
            if lp < cp {
                return false;
            }
        }
        false
    }

    fn parse_version(version: &str) -> Vec<u32> {
        let trimmed = version
            .split(['-', '+'])
            .next()
            .unwrap_or(version);
        trimmed
            .split('.')
            .filter_map(|part| {
                let mut numeric = String::new();
                for ch in part.chars() {
                    if ch.is_ascii_digit() {
                        numeric.push(ch);
                    } else {
                        break;
                    }
                }
                if numeric.is_empty() {
                    None
                } else {
                    numeric.parse().ok()
                }
            })
            .collect()
    }

    fn download_and_install(&self) -> Result<(), Box<dyn std::error::Error>> {
        let release = self.release.as_ref().ok_or("release not loaded")?;
        let asset = self.find_asset(release)?;
        println!("Downloading {}...", asset.name);

        let download_path = self.cfg.work_dir.join(&asset.name);
        self.download_file(&asset.browser_download_url, &download_path)?;
        let _cleanup = TempFileCleanup {
            path: download_path.clone(),
        };

        if let Some(checksum_asset) = self.find_checksum_asset(release) {
            println!("Verifying checksum...");
            self.verify_checksum(&download_path, &checksum_asset, &asset.name)?;
            println!("Checksum verified.");
        }

        let is_portable = self.cfg.is_portable() || self.opts.portable;
        if is_portable || asset.name.to_lowercase().ends_with(".zip") {
            println!("Extracting...");
            let result = self.extract_portable(&download_path);
            let _ = fs::remove_file(&download_path);
            return result;
        }

        println!("Installing...");
        let result = self.run_installer(&download_path);
        let _ = fs::remove_file(&download_path);
        result?;
        Ok(())
    }

    fn find_asset(&self, release: &Release) -> Result<Asset, Box<dyn std::error::Error>> {
        let is_portable = self.cfg.is_portable() || self.opts.portable;
        let arch = if cfg!(target_arch = "x86") {
            "i686"
        } else {
            "x86_64"
        };

        let suffix = if is_portable {
            format!("windows-{arch}-portable.zip")
        } else {
            format!("windows-{arch}-setup.exe")
        };
        let suffixes = vec![
            suffix,
            "win64.zip".to_string(),
            "win64-setup.exe".to_string(),
            "windows.zip".to_string(),
            "windows-setup.exe".to_string(),
        ];

        for asset in &release.assets {
            let name = asset.name.to_lowercase();
            for s in &suffixes {
                if name.contains(&s.to_lowercase()) || name.ends_with(&s.to_lowercase()) {
                    return Ok(asset.clone());
                }
            }
        }

        for asset in &release.assets {
            let name = asset.name.to_lowercase();
            if (name.contains("windows") || name.contains("win"))
                && (name.ends_with(".exe") || name.ends_with(".zip"))
            {
                return Ok(asset.clone());
            }
        }

        Err("no suitable download found for this platform".into())
    }

    fn find_checksum_asset(&self, release: &Release) -> Option<Asset> {
        release.assets.iter().find_map(|asset| {
            let name = asset.name.to_lowercase();
            if name.contains("sha256") || name.ends_with(".sha256") {
                Some(asset.clone())
            } else {
                None
            }
        })
    }

    fn download_file(&self, url: &str, path: &Path) -> Result<(), Box<dyn std::error::Error>> {
        let response = ureq::get(url)
            .set("User-Agent", &format!("Noraneko-WinUpdater/{}", self.opts.version))
            .timeout(Duration::from_secs(300))
            .call();
        let response = match response {
            Ok(response) => response,
            Err(ureq::Error::Status(code, _)) => {
                return Err(format!("download returned status {}", code).into());
            }
            Err(err) => return Err(err.into()),
        };
        if response.status() != 200 {
            return Err(format!("download returned status {}", response.status()).into());
        }
        let mut reader = response.into_reader();
        let mut out = File::create(path)?;
        io::copy(&mut reader, &mut out)?;
        Ok(())
    }

    fn verify_checksum(
        &self,
        file_path: &Path,
        checksum_asset: &Asset,
        file_name: &str,
    ) -> Result<(), Box<dyn std::error::Error>> {
        let checksum_path = self.cfg.work_dir.join(&checksum_asset.name);
        self.download_file(&checksum_asset.browser_download_url, &checksum_path)?;
        let data = fs::read_to_string(&checksum_path)?;
        let _ = fs::remove_file(&checksum_path);

        let mut expected_hash = String::new();
        for line in data.lines() {
            let parts: Vec<&str> = line.split_whitespace().collect();
            if parts.len() >= 2 {
                let hash = parts[0];
                let name = parts[1].trim_start_matches('*');
                if name.eq_ignore_ascii_case(file_name) || name.ends_with(file_name) {
                    expected_hash = hash.to_lowercase();
                    break;
                }
            }
        }
        if expected_hash.is_empty() {
            return Err(format!("checksum for {file_name} not found in checksum file").into());
        }

        let mut file = File::open(file_path)?;
        let mut hasher = Sha256::new();
        let mut buffer = [0u8; 8192];
        loop {
            let count = file.read(&mut buffer)?;
            if count == 0 {
                break;
            }
            hasher.update(&buffer[..count]);
        }
        let actual_hash = format!("{:x}", hasher.finalize());
        if actual_hash != expected_hash {
            return Err(format!("checksum mismatch: expected {expected_hash}, got {actual_hash}").into());
        }
        Ok(())
    }

    fn extract_portable(&self, zip_path: &Path) -> Result<(), Box<dyn std::error::Error>> {
        let browser_dir = {
            let browser_path = self.cfg.get_browser_path();
            let path = Path::new(&browser_path)
                .parent()
                .map(PathBuf::from)
                .unwrap_or_else(|| self.cfg.exe_dir.join(config::BROWSER_NAME));
            path
        };

        let extract_dir = self
            .cfg
            .work_dir
            .join(format!("{}-Extracted", config::BROWSER_NAME));
        if extract_dir.exists() {
            fs::remove_dir_all(&extract_dir)?;
        }
        fs::create_dir_all(&extract_dir)?;

        self.unzip(zip_path, &extract_dir)?;

        let mut source_dir = extract_dir.clone();
        for entry in fs::read_dir(&extract_dir)? {
            let entry = entry?;
            if entry.file_type()?.is_dir() {
                source_dir = entry.path();
                break;
            }
        }

        self.copy_dir(&source_dir, &browser_dir)?;
        fs::remove_dir_all(&extract_dir)?;
        Ok(())
    }

    fn unzip(&self, src: &Path, dest: &Path) -> Result<(), Box<dyn std::error::Error>> {
        let file = File::open(src)?;
        let mut archive = ZipArchive::new(file)?;
        let dest = dest.to_path_buf();

        for i in 0..archive.len() {
            let mut file = archive.by_index(i)?;
            let name = file.name().to_string();
            let clean_name = Path::new(&name);
            if clean_name
                .components()
                .any(|component| matches!(component, std::path::Component::ParentDir))
                || clean_name.is_absolute()
            {
                return Err(format!("illegal file path in archive: {name}").into());
            }
            let fpath = dest.join(clean_name);
            if !fpath.starts_with(&dest) && fpath != dest {
                return Err(format!("illegal file path: {}", fpath.display()).into());
            }

            if file.is_dir() {
                fs::create_dir_all(&fpath)?;
                continue;
            }

            if let Some(parent) = fpath.parent() {
                fs::create_dir_all(parent)?;
            }
            let mut out_file = File::create(&fpath)?;
            io::copy(&mut file, &mut out_file)?;
        }
        Ok(())
    }

    fn copy_dir(&self, src: &Path, dst: &Path) -> Result<(), Box<dyn std::error::Error>> {
        for entry in WalkDir::new(src) {
            let entry = entry?;
            let path = entry.path();
            let rel_path = path.strip_prefix(src)?;
            let dest_path = dst.join(rel_path);
            if entry.file_type().is_dir() {
                fs::create_dir_all(&dest_path)?;
            } else {
                self.copy_file(path, &dest_path)?;
            }
        }
        Ok(())
    }

    fn copy_file(&self, src: &Path, dst: &Path) -> Result<(), Box<dyn std::error::Error>> {
        let mut source = File::open(src)?;
        let mut dest = File::create(dst)?;
        io::copy(&mut source, &mut dest)?;
        Ok(())
    }

    fn run_installer(&self, setup_path: &Path) -> Result<(), Box<dyn std::error::Error>> {
        let browser_path = self.cfg.get_browser_path();
        let browser_dir = Path::new(&browser_path)
            .parent()
            .map(PathBuf::from)
            .unwrap_or_else(|| {
                let program_files =
                    std::env::var("ProgramFiles").unwrap_or_else(|_| "C:\\Program Files".to_string());
                PathBuf::from(program_files).join(config::BROWSER_NAME)
            });

        let status = Command::new(setup_path)
            .arg("/S")
            .arg(format!("/D={}", browser_dir.display()))
            .status();

        match status {
            Ok(status) if status.success() => Ok(()),
            _ => {
                println!("Silent installation failed, running interactive installer...");
                let status = Command::new(setup_path)
                    .arg(format!("/D={}", browser_dir.display()))
                    .status()?;
                if status.success() {
                    Ok(())
                } else {
                    Err("installer failed".into())
                }
            }
        }
    }

    fn log_result(&self, result: &str) {
        let timestamp = OffsetDateTime::now_utc()
            .format(&format_description!("[year]-[month]-[day] [hour]:[minute]:[second]"))
            .unwrap_or_else(|_| "1970-01-01 00:00:00".to_string());

        let _ = self.cfg.log_entry("LastRun", &timestamp);
        let _ = self.cfg.log_entry("LastResult", result);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn temp_config() -> (config::Config, tempfile::TempDir) {
        let dir = tempfile::tempdir().expect("temp dir");
        let cfg = config::Config {
            path: String::new(),
            work_dir: dir.path().to_path_buf(),
            update_self: true,
            ignore_crl_errors: false,
            branch: "nightly".to_string(),
            exe_dir: dir.path().to_path_buf(),
            config_file: dir.path().join(config::CONFIG_FILE_NAME),
        };
        (cfg, dir)
    }

    #[test]
    fn test_new() {
        let (cfg, _dir) = temp_config();
        let opts = Options {
            scheduled: false,
            portable: false,
            check_only: true,
            create_task: false,
            remove_task: false,
            version: "1.0.0".to_string(),
        };
        let updater = Updater::new(cfg.clone(), opts);
        assert_eq!(updater.opts.version, "1.0.0");
        assert_eq!(updater.cfg.exe_dir, cfg.exe_dir);
    }

    #[test]
    fn test_is_newer_version() {
        let cases = vec![
            ("1.0.0", "1.0.1", true),
            ("1.0.0", "1.0.0", false),
            ("", "1.0.0", true),
            ("0.0.0", "1.0.0", true),
            ("v1.0.0", "v1.0.1", true),
            ("v1.0.0", "1.0.1", true),
            ("1.0.0", "1.1.0", true),
            ("1.1.0", "1.0.1", false),
            ("1.0.0", "2.0.0", true),
            ("2.0.0", "1.9.9", false),
            ("1.0.0-beta", "1.0.0", false),
            ("1.10.0", "1.9.0", false),
            ("1.2.3", "1.2.4", true),
            ("1.2.4", "1.2.3", false),
        ];

        for (current, latest, expected) in cases {
            assert_eq!(Updater::is_newer_version(current, latest), expected);
        }
    }

    #[test]
    fn test_unzip_invalid() {
        let (cfg, _dir) = temp_config();
        let updater = Updater::new(cfg, Options {
            scheduled: false,
            portable: false,
            check_only: false,
            create_task: false,
            remove_task: false,
            version: "1.0.0".to_string(),
        });
        let invalid_zip = updater.cfg.work_dir.join("invalid.zip");
        fs::write(&invalid_zip, b"not a zip file").unwrap();
        let dest_dir = updater.cfg.work_dir.join("extract");
        fs::create_dir_all(&dest_dir).unwrap();
        assert!(updater.unzip(&invalid_zip, &dest_dir).is_err());
    }

    #[test]
    fn test_copy_file() {
        let (cfg, _dir) = temp_config();
        let updater = Updater::new(cfg, Options {
            scheduled: false,
            portable: false,
            check_only: false,
            create_task: false,
            remove_task: false,
            version: "1.0.0".to_string(),
        });
        let src = updater.cfg.work_dir.join("source.txt");
        let dst = updater.cfg.work_dir.join("dest.txt");
        fs::write(&src, b"Hello, World!").unwrap();
        updater.copy_file(&src, &dst).unwrap();
        let content = fs::read_to_string(&dst).unwrap();
        assert_eq!(content, "Hello, World!");
    }

    #[test]
    fn test_find_asset() {
        let (cfg, _dir) = temp_config();
        let updater = Updater::new(cfg.clone(), Options {
            scheduled: false,
            portable: true,
            check_only: false,
            create_task: false,
            remove_task: false,
            version: "1.0.0".to_string(),
        });
        let release = Release {
            tag_name: "v1.0.0".to_string(),
            assets: vec![
                Asset {
                    name: "noraneko-1.0.0-linux-x86_64.tar.gz".to_string(),
                    browser_download_url: "https://example.com/linux.tar.gz".to_string(),
                },
                Asset {
                    name: "noraneko-1.0.0-windows-x86_64-portable.zip".to_string(),
                    browser_download_url: "https://example.com/win.zip".to_string(),
                },
                Asset {
                    name: "noraneko-1.0.0-windows-x86_64-setup.exe".to_string(),
                    browser_download_url: "https://example.com/setup.exe".to_string(),
                },
            ],
        };
        let asset = updater.find_asset(&release).unwrap();
        assert_eq!(asset.name, "noraneko-1.0.0-windows-x86_64-portable.zip");

        let updater_installed = Updater::new(cfg, Options {
            scheduled: false,
            portable: false,
            check_only: false,
            create_task: false,
            remove_task: false,
            version: "1.0.0".to_string(),
        });
        let asset_installed = updater_installed.find_asset(&release).unwrap();
        assert!(!asset_installed.name.is_empty());
    }

    #[test]
    fn test_find_checksum_asset() {
        let (cfg, _dir) = temp_config();
        let updater = Updater::new(cfg, Options {
            scheduled: false,
            portable: false,
            check_only: false,
            create_task: false,
            remove_task: false,
            version: "1.0.0".to_string(),
        });
        let release = Release {
            tag_name: "v1.0.0".to_string(),
            assets: vec![
                Asset {
                    name: "noraneko-1.0.0-windows.zip".to_string(),
                    browser_download_url: "https://example.com/win.zip".to_string(),
                },
                Asset {
                    name: "sha256sums.txt".to_string(),
                    browser_download_url: "https://example.com/sha256sums.txt".to_string(),
                },
            ],
        };
        let checksum = updater.find_checksum_asset(&release).unwrap();
        assert_eq!(checksum.name, "sha256sums.txt");

        let release_sha = Release {
            tag_name: "v1.0.0".to_string(),
            assets: vec![
                Asset {
                    name: "noraneko-1.0.0-windows.zip".to_string(),
                    browser_download_url: "https://example.com/win.zip".to_string(),
                },
                Asset {
                    name: "noraneko-1.0.0-windows.zip.sha256".to_string(),
                    browser_download_url: "https://example.com/checksum.sha256".to_string(),
                },
            ],
        };
        assert!(updater.find_checksum_asset(&release_sha).is_some());
    }
}
