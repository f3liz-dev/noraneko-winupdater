use std::env;
use std::fs;
use std::io::{self, BufRead};
use std::path::{Path, PathBuf};

pub const BROWSER_NAME: &str = "Noraneko";
pub const BROWSER_EXE: &str = "noraneko.exe";
pub const DEFAULT_BRANCH: &str = "nightly";
pub const CONFIG_FILE_NAME: &str = "Noraneko-WinUpdater.ini";
pub const RELEASE_API_URL: &str = "https://api.github.com/repos/f3liz-dev/noraneko-runtime/releases";
pub const CONNECT_CHECK_URL: &str = "https://api.github.com";

#[derive(Clone, Debug)]
pub struct Config {
    pub path: String,
    pub work_dir: PathBuf,
    pub update_self: bool,
    pub ignore_crl_errors: bool,
    pub branch: String,
    pub exe_dir: PathBuf,
    pub config_file: PathBuf,
}

impl Config {
    pub fn load(exe_dir: &Path) -> Result<Self, io::Error> {
        let exe_dir = exe_dir.to_path_buf();
        let mut cfg = Config {
            path: String::new(),
            work_dir: env::temp_dir(),
            update_self: true,
            ignore_crl_errors: false,
            branch: DEFAULT_BRANCH.to_string(),
            exe_dir: exe_dir.clone(),
            config_file: exe_dir.join(CONFIG_FILE_NAME),
        };

        if !cfg.config_file.exists() {
            cfg.save()?;
            return Ok(cfg);
        }

        let file = fs::File::open(&cfg.config_file)?;
        let reader = io::BufReader::new(file);
        let mut section = String::new();

        for line in reader.lines() {
            let line = line?;
            let trimmed = line.trim();
            if trimmed.is_empty() || trimmed.starts_with(';') || trimmed.starts_with('#') {
                continue;
            }
            if trimmed.starts_with('[') && trimmed.ends_with(']') {
                section = trimmed[1..trimmed.len() - 1].to_lowercase();
                continue;
            }
            let mut parts = trimmed.splitn(2, '=');
            let key = parts.next().unwrap_or("").trim().to_lowercase();
            let value = parts.next().unwrap_or("").trim().to_string();
            if section == "settings" {
                match key.as_str() {
                    "path" => {
                        if value != "0" && !value.is_empty() {
                            cfg.path = value;
                        }
                    }
                    "workdir" => {
                        if !value.is_empty() {
                            if value == "." {
                                cfg.work_dir = exe_dir.clone();
                            } else {
                                cfg.work_dir = PathBuf::from(value);
                            }
                        }
                    }
                    "updateself" => {
                        cfg.update_self = value == "1" || value.eq_ignore_ascii_case("true");
                    }
                    "ignorecrlerrors" => {
                        cfg.ignore_crl_errors = value == "1" || value.eq_ignore_ascii_case("true");
                    }
                    "branch" => {
                        if !value.is_empty() {
                            cfg.branch = value;
                        }
                    }
                    _ => {}
                }
            }
        }

        Ok(cfg)
    }

    pub fn save(&self) -> Result<(), io::Error> {
        let mut content = String::new();
        content.push_str("[Settings]\n");
        if self.path.is_empty() {
            content.push_str("Path=0\n");
        } else {
            content.push_str(&format!("Path={}\n", self.path));
        }

        let work_dir = if self.work_dir == self.exe_dir {
            ".".to_string()
        } else if self.work_dir == env::temp_dir() {
            "".to_string()
        } else {
            self.work_dir.to_string_lossy().to_string()
        };
        content.push_str(&format!("WorkDir={}\n", work_dir));

        content.push_str(if self.update_self {
            "UpdateSelf=1\n"
        } else {
            "UpdateSelf=0\n"
        });

        content.push_str(if self.ignore_crl_errors {
            "IgnoreCrlErrors=1\n"
        } else {
            "IgnoreCrlErrors=0\n"
        });

        content.push_str(&format!("Branch={}\n", self.branch));
        fs::write(&self.config_file, content)
    }

    pub fn log_entry(&self, key: &str, value: &str) -> Result<(), io::Error> {
        let mut existing_content = fs::read_to_string(&self.config_file).unwrap_or_default();
        if !existing_content.contains("[Log]") {
            existing_content.push_str("\n[Log]\n");
        }

        let mut lines: Vec<String> = existing_content.lines().map(|line| line.to_string()).collect();
        let mut found = false;
        let mut in_log_section = false;
        for line in &mut lines {
            let trimmed = line.trim();
            if trimmed.eq_ignore_ascii_case("[Log]") {
                in_log_section = true;
                continue;
            }
            if trimmed.starts_with('[') && trimmed.ends_with(']') {
                in_log_section = false;
                continue;
            }
            if in_log_section && trimmed.to_lowercase().starts_with(&format!("{}=", key.to_lowercase())) {
                *line = format!("{key}={value}");
                found = true;
                break;
            }
        }

        if !found {
            let mut new_lines = Vec::new();
            let mut added_to_log = false;
            in_log_section = false;
            for line in &lines {
                let trimmed = line.trim();
                if trimmed.eq_ignore_ascii_case("[Log]") {
                    in_log_section = true;
                    new_lines.push(line.clone());
                    continue;
                }
                if in_log_section
                    && !added_to_log
                    && (trimmed.is_empty() || (trimmed.starts_with('[') && trimmed.ends_with(']')))
                {
                    new_lines.push(format!("{key}={value}"));
                    added_to_log = true;
                }
                if trimmed.starts_with('[') && trimmed.ends_with(']') && !trimmed.eq_ignore_ascii_case("[Log]") {
                    in_log_section = false;
                }
                new_lines.push(line.clone());
            }
            if !added_to_log {
                new_lines.push(format!("{key}={value}"));
            }
            lines = new_lines;
        }

        fs::write(&self.config_file, lines.join("\n"))
    }

    pub fn get_browser_path(&self) -> String {
        if !self.path.is_empty() {
            return self.path.clone();
        }

        let program_files = env::var("ProgramFiles").unwrap_or_else(|_| "C:\\Program Files".to_string());
        let mut possible_paths = vec![
            self.exe_dir.join(BROWSER_NAME).join(BROWSER_EXE),
            PathBuf::from(program_files).join(BROWSER_NAME).join(BROWSER_EXE),
        ];

        let portable_path = self.exe_dir.join(format!("{BROWSER_NAME}-Portable.exe"));
        if portable_path.exists() {
            possible_paths.insert(0, self.exe_dir.join(BROWSER_NAME).join(BROWSER_EXE));
        }

        for path in possible_paths {
            if path.exists() {
                return path.to_string_lossy().to_string();
            }
        }

        String::new()
    }

    pub fn is_portable(&self) -> bool {
        let portable_path = self.exe_dir.join(format!("{BROWSER_NAME}-Portable.exe"));
        portable_path.exists()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_temp_dir() -> tempfile::TempDir {
        tempfile::tempdir().expect("temp dir")
    }

    #[test]
    fn test_load_defaults() {
        let temp_dir = create_temp_dir();
        let cfg = Config::load(temp_dir.path()).expect("load config");
        assert_eq!(cfg.branch, DEFAULT_BRANCH);
        assert!(cfg.update_self);
        assert!(!cfg.ignore_crl_errors);
        assert!(cfg.config_file.exists());
    }

    #[test]
    fn test_load_existing_config() {
        let temp_dir = create_temp_dir();
        let config_content = r"[Settings]
Path=C:\Program Files\Noraneko\noraneko.exe
WorkDir=D:\Temp
UpdateSelf=0
IgnoreCrlErrors=1
Branch=beta
";
        let config_path = temp_dir.path().join(CONFIG_FILE_NAME);
        fs::write(&config_path, config_content).expect("write config");

        let cfg = Config::load(temp_dir.path()).expect("load config");
        assert_eq!(cfg.path, r"C:\Program Files\Noraneko\noraneko.exe");
        assert_eq!(cfg.work_dir, PathBuf::from(r"D:\Temp"));
        assert!(!cfg.update_self);
        assert!(cfg.ignore_crl_errors);
        assert_eq!(cfg.branch, "beta");
    }

    #[test]
    fn test_save() {
        let temp_dir = create_temp_dir();
        let cfg = Config {
            path: r"C:\Test\noraneko.exe".to_string(),
            work_dir: PathBuf::from(r"D:\Temp"),
            update_self: false,
            ignore_crl_errors: true,
            branch: "stable".to_string(),
            exe_dir: temp_dir.path().to_path_buf(),
            config_file: temp_dir.path().join(CONFIG_FILE_NAME),
        };

        cfg.save().expect("save config");
        let content = fs::read_to_string(&cfg.config_file).expect("read config");
        assert!(content.contains(r"Path=C:\Test\noraneko.exe"));
        assert!(content.contains("UpdateSelf=0"));
        assert!(content.contains("IgnoreCrlErrors=1"));
        assert!(content.contains("Branch=stable"));
    }

    #[test]
    fn test_log_entry() {
        let temp_dir = create_temp_dir();
        let cfg = Config::load(temp_dir.path()).expect("load config");
        cfg.log_entry("LastRun", "2024-01-01 12:00:00")
            .expect("log entry");
        cfg.log_entry("LastResult", "No new version found")
            .expect("log entry");

        let content = fs::read_to_string(&cfg.config_file).expect("read config");
        assert!(content.contains("[Log]"));
        assert!(content.contains("LastRun=2024-01-01 12:00:00"));
        assert!(content.contains("LastResult=No new version found"));
    }
}
