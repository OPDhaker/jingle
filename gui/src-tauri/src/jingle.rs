//! The only place the GUI talks to jingle. Every read and write goes through
//! the installed `jingle` CLI with `--json` (docs/json.md), so the GUI holds
//! no logic of its own and always agrees with the terminal.

use std::env;
use std::ffi::OsString;
use std::io::Read;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::thread;
use std::time::{Duration, Instant};

use serde::Serialize;
use serde_json::Value;

/// How long one jingle command may run. Every command is quick; this only
/// guards against a hung git or player.
const RUN_TIMEOUT: Duration = Duration::from_secs(15);
/// How long the login shell gets to report where jingle is.
const SHELL_TIMEOUT: Duration = Duration::from_secs(3);

/// GUI-side error codes. They are not CLI codes, hence the `cli_` prefix.
pub const CODE_NOT_FOUND: &str = "cli_not_found";
pub const CODE_SPAWN: &str = "cli_spawn_failed";
pub const CODE_TIMEOUT: &str = "cli_timeout";
pub const CODE_BAD_OUTPUT: &str = "cli_bad_output";
/// The installed jingle is older than this app needs.
pub const CODE_CLI_OLD: &str = "cli_too_old";
/// The installed jingle's JSON is newer than this app understands.
pub const CODE_APP_OLD: &str = "cli_too_new";

/// The `schema_version` of docs/json.md this app was written against.
pub const SCHEMA_VERSION: u64 = 1;

/// A failed jingle command: the CLI's error envelope, or a `cli_*` code when
/// the CLI couldn't be run or answered with something that isn't JSON.
#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct JingleError {
    pub code: String,
    pub message: String,
    /// The CLI's exit code; -1 when it never exited normally.
    pub exit: i32,
}

impl JingleError {
    pub fn new(code: &str, message: impl Into<String>) -> Self {
        JingleError { code: code.into(), message: message.into(), exit: -1 }
    }
}

/// A located jingle binary and the PATH its subprocesses get.
#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Jingle {
    pub bin: PathBuf,
    /// Apps started from Finder or a desktop launcher get a bare PATH; jingle
    /// needs git and the audio player, so it runs with the login shell's.
    #[serde(skip)]
    pub path_env: OsString,
}

/// Finds jingle: `JINGLE_BIN` if set (and nothing else, so a bad override is
/// reported instead of hidden), else the login shell's `command -v jingle`,
/// else the usual install dirs.
pub fn locate() -> Result<Jingle, JingleError> {
    let home = env::var_os("HOME").map(PathBuf::from);
    locate_with(
        env::var_os("JINGLE_BIN"),
        login_shell_lookup,
        &candidate_dirs(home.as_deref()),
        env::var_os("PATH").unwrap_or_default(),
    )
}

/// What the login shell reported: jingle's path (if any) and its PATH.
pub struct ShellLookup {
    pub bin: Option<PathBuf>,
    pub path: OsString,
}

pub fn locate_with(
    override_bin: Option<OsString>,
    shell: impl FnOnce() -> Option<ShellLookup>,
    candidates: &[PathBuf],
    base_path: OsString,
) -> Result<Jingle, JingleError> {
    if let Some(bin) = override_bin.filter(|b| !b.is_empty()) {
        let bin = PathBuf::from(bin);
        if !is_executable(&bin) {
            return Err(JingleError::new(
                CODE_NOT_FOUND,
                format!("JINGLE_BIN is set to {}, which is not an executable file", bin.display()),
            ));
        }
        return Ok(Jingle { path_env: with_dir(&base_path, &bin), bin });
    }
    let shell = shell();
    let path = shell.as_ref().map(|s| s.path.clone()).filter(|p| !p.is_empty()).unwrap_or(base_path);
    let found = shell
        .and_then(|s| s.bin)
        .filter(|b| is_executable(b))
        .or_else(|| candidates.iter().map(|d| d.join("jingle")).find(|b| is_executable(b)));
    match found {
        Some(bin) => Ok(Jingle { path_env: with_dir(&path, &bin), bin }),
        None => Err(JingleError::new(CODE_NOT_FOUND, "the jingle command-line tool isn't installed")),
    }
}

/// Dirs jingle is commonly installed to: Homebrew, install.sh, `go install`.
pub fn candidate_dirs(home: Option<&Path>) -> Vec<PathBuf> {
    let mut dirs: Vec<PathBuf> =
        ["/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"].iter().map(PathBuf::from).collect();
    if let Some(home) = home {
        dirs.push(home.join(".local/bin"));
        dirs.push(home.join("go/bin"));
    }
    dirs
}

/// Asks the user's login shell where jingle is and what PATH it has, the
/// way a terminal would see them. Interactive too, since many people set
/// PATH in .zshrc / .bashrc.
fn login_shell_lookup() -> Option<ShellLookup> {
    let shell = env::var_os("SHELL").filter(|s| !s.is_empty()).unwrap_or_else(|| "/bin/sh".into());
    let script = r#"printf '\n__JINGLE_BIN__=%s\n__JINGLE_PATH__=%s\n' "$(command -v jingle 2>/dev/null)" "$PATH""#;
    let mut cmd = Command::new(shell);
    cmd.args(["-ilc", script]);
    let out = run_with_timeout(cmd, SHELL_TIMEOUT).ok()?;
    parse_shell_lookup(&out.stdout)
}

pub fn parse_shell_lookup(stdout: &[u8]) -> Option<ShellLookup> {
    let text = String::from_utf8_lossy(stdout);
    let mut bin = None;
    let mut path = None;
    for line in text.lines() {
        if let Some(v) = line.strip_prefix("__JINGLE_BIN__=") {
            bin = Some(v.trim().to_string()).filter(|v| v.starts_with('/')).map(PathBuf::from);
        } else if let Some(v) = line.strip_prefix("__JINGLE_PATH__=") {
            path = Some(OsString::from(v.trim()));
        }
    }
    Some(ShellLookup { bin, path: path? })
}

/// PATH with bin's dir appended when it's missing, so jingle's hooks and
/// any `jingle` it looks up resolve to the same binary.
fn with_dir(path: &OsString, bin: &Path) -> OsString {
    let Some(dir) = bin.parent() else { return path.clone() };
    let mut dirs: Vec<PathBuf> = env::split_paths(path).collect();
    if !dirs.iter().any(|d| d == dir) {
        dirs.push(dir.to_path_buf());
    }
    env::join_paths(dirs).unwrap_or_else(|_| path.clone())
}

fn is_executable(p: &Path) -> bool {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        p.metadata().map(|m| m.is_file() && m.permissions().mode() & 0o111 != 0).unwrap_or(false)
    }
    #[cfg(not(unix))]
    {
        p.is_file()
    }
}

impl Jingle {
    /// Runs `jingle <cmd> --json -- <positional>` and returns its result
    /// object. Positional args come after `--`, so a value like a file named
    /// `-x.mp3` can never be read as a flag. On failure it returns the CLI's
    /// error envelope. `doctor` is special: it exits 1 with `problems_found`
    /// but still prints its report, which is returned as a success because
    /// the report is what the caller wants.
    pub fn run(&self, cmd: &[&str], positional: &[&str]) -> Result<Value, JingleError> {
        let mut c = Command::new(&self.bin);
        c.args(cmd).arg("--json").env("PATH", &self.path_env);
        if !positional.is_empty() {
            c.arg("--").args(positional);
        }
        let cmd = c;
        let out = run_with_timeout(cmd, RUN_TIMEOUT)?;
        parse_output(out)
    }

    /// Checks that this jingle has everything the app uses. `jingle schema`
    /// came in Phase 4 and `sound_types` (which marks `jingle preview`) in
    /// Phase 5, so an older jingle fails here instead of half working.
    pub fn check_compatible(&self) -> Result<(), JingleError> {
        let too_old = |version: &str| {
            JingleError::new(
                CODE_CLI_OLD,
                format!("jingle {version} at {} is too old for this app.", self.bin.display()),
            )
        };
        let schema = match self.run(&["schema"], &[]) {
            Ok(s) => s,
            Err(e) if e.code.starts_with("cli_") => return Err(e),
            // No `jingle schema` at all; `jingle version` still names it.
            Err(_) => {
                let v = self.run(&["version"], &[]).ok();
                let version = v.as_ref().and_then(|v| v.get("version")).and_then(Value::as_str);
                return Err(too_old(version.unwrap_or("(unknown version)")));
            }
        };
        let version = schema.get("version").and_then(Value::as_str).unwrap_or("(unknown version)");
        let got = schema.get("schema_version").and_then(Value::as_u64).unwrap_or(0);
        if got > SCHEMA_VERSION {
            return Err(JingleError::new(
                CODE_APP_OLD,
                format!(
                    "jingle {version} uses a newer JSON schema ({got}) than this app understands ({SCHEMA_VERSION})."
                ),
            ));
        }
        if got < SCHEMA_VERSION || schema.get("sound_types").is_none() {
            return Err(too_old(version));
        }
        Ok(())
    }
}

pub struct Output {
    pub code: Option<i32>,
    pub stdout: Vec<u8>,
    pub stderr: Vec<u8>,
}

pub fn parse_output(out: Output) -> Result<Value, JingleError> {
    let exit = out.code.unwrap_or(-1);
    let stdout: Option<Value> = serde_json::from_slice(&out.stdout).ok();
    if exit == 0 {
        return stdout.ok_or_else(|| bad_output(exit, &out.stdout));
    }
    let envelope = serde_json::from_slice::<Value>(&out.stderr).ok().and_then(|v| {
        let e = v.get("error")?;
        Some(JingleError {
            code: e.get("code")?.as_str()?.to_string(),
            message: e.get("message").and_then(Value::as_str).unwrap_or_default().to_string(),
            exit,
        })
    });
    match (envelope, stdout) {
        (Some(e), Some(report)) if e.code == "problems_found" => Ok(report),
        (Some(e), _) => Err(e),
        (None, _) => Err(bad_output(exit, &out.stderr)),
    }
}

fn bad_output(exit: i32, text: &[u8]) -> JingleError {
    let text = String::from_utf8_lossy(text);
    let text = text.trim();
    JingleError {
        code: CODE_BAD_OUTPUT.into(),
        message: if text.is_empty() {
            format!("jingle exited {exit} with no JSON output")
        } else {
            format!("jingle exited {exit}: {}", text.chars().take(500).collect::<String>())
        },
        exit,
    }
}

/// Runs cmd with null stdin, collecting stdout and stderr, and kills it at
/// the deadline.
pub fn run_with_timeout(mut cmd: Command, timeout: Duration) -> Result<Output, JingleError> {
    let mut child = cmd
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| JingleError::new(CODE_SPAWN, format!("couldn't run {:?}: {e}", cmd.get_program())))?;
    // Read both pipes on their own threads so a full pipe can't stall the child.
    let drain = |r: Option<Box<dyn Read + Send>>| {
        thread::spawn(move || {
            let mut buf = Vec::new();
            if let Some(mut r) = r {
                let _ = r.read_to_end(&mut buf);
            }
            buf
        })
    };
    let out = drain(child.stdout.take().map(|r| Box::new(r) as Box<dyn Read + Send>));
    let err = drain(child.stderr.take().map(|r| Box::new(r) as Box<dyn Read + Send>));
    let deadline = Instant::now() + timeout;
    let status = loop {
        match child.try_wait() {
            Ok(Some(status)) => break status,
            Ok(None) if Instant::now() < deadline => thread::sleep(Duration::from_millis(10)),
            Ok(None) => {
                let _ = child.kill();
                let _ = child.wait();
                return Err(JingleError::new(
                    CODE_TIMEOUT,
                    format!("{:?} didn't finish within {}s", cmd.get_program(), timeout.as_secs()),
                ));
            }
            Err(e) => return Err(JingleError::new(CODE_SPAWN, e.to_string())),
        }
    };
    Ok(Output { code: status.code(), stdout: out.join().unwrap_or_default(), stderr: err.join().unwrap_or_default() })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::os::unix::fs::PermissionsExt;

    fn script(dir: &Path, name: &str, body: &str) -> PathBuf {
        let p = dir.join(name);
        fs::write(&p, format!("#!/bin/sh\n{body}\n")).unwrap();
        fs::set_permissions(&p, fs::Permissions::from_mode(0o755)).unwrap();
        p
    }

    fn fake(body: &str) -> (tempfile::TempDir, Jingle) {
        let dir = tempfile::tempdir().unwrap();
        let bin = script(dir.path(), "jingle", body);
        let path_env = env::var_os("PATH").unwrap();
        (dir, Jingle { bin, path_env })
    }

    #[test]
    fn run_returns_stdout_json_and_passes_json_flag() {
        let (_d, j) = fake(r#"printf '{"args": "%s"}' "$*""#);
        assert_eq!(j.run(&["status"], &[]).unwrap()["args"], "status --json");
        let got = j.run(&["config", "set"], &["commit.sounds", "-x.mp3"]).unwrap();
        assert_eq!(got["args"], "config set --json -- commit.sounds -x.mp3");
    }

    #[test]
    fn run_returns_error_envelope() {
        let (_d, j) = fake(r#"echo '{"error": {"code": "invalid_key", "message": "nope"}}' >&2; exit 2"#);
        let e = j.run(&["config", "get"], &["x"]).unwrap_err();
        assert_eq!(e, JingleError { code: "invalid_key".into(), message: "nope".into(), exit: 2 });
    }

    #[test]
    fn doctor_report_with_problems_is_a_result() {
        let (_d, j) = fake(
            r#"echo '{"problems": [{"code": "not_installed"}]}'
echo '{"error": {"code": "problems_found", "message": "1 problem"}}' >&2; exit 1"#,
        );
        assert_eq!(j.run(&["doctor"], &[]).unwrap()["problems"][0]["code"], "not_installed");
    }

    #[test]
    fn garbage_is_bad_output() {
        let (_d, j) = fake("echo 'panic: oops' >&2; exit 2");
        let e = j.run(&["status"], &[]).unwrap_err();
        assert_eq!((e.code.as_str(), e.exit), (CODE_BAD_OUTPUT, 2));
        assert!(e.message.contains("panic: oops"));
        let (_d, j) = fake("echo 'not json'");
        assert_eq!(j.run(&["status"], &[]).unwrap_err().code, CODE_BAD_OUTPUT);
    }

    #[test]
    fn hung_command_times_out() {
        let dir = tempfile::tempdir().unwrap();
        let bin = script(dir.path(), "slow", "sleep 5");
        let start = Instant::now();
        let e = run_with_timeout(Command::new(bin), Duration::from_millis(200)).err().unwrap();
        assert_eq!(e.code, CODE_TIMEOUT);
        assert!(start.elapsed() < Duration::from_secs(3));
    }

    #[test]
    fn missing_binary_is_spawn_error() {
        let e = run_with_timeout(Command::new("/no/such/jingle"), RUN_TIMEOUT).err().unwrap();
        assert_eq!(e.code, CODE_SPAWN);
    }

    #[test]
    fn compatibility() {
        let current = r#"echo '{"schema_version": 1, "version": "v0.1.0", "sound_types": [".wav"]}'"#;
        let (_d, j) = fake(current);
        assert_eq!(j.check_compatible(), Ok(()));

        // Before `jingle schema`: cobra's usage error.
        let (_d, j) = fake(
            r#"[ "$1" = version ] && { echo '{"version": "v0.0.1"}'; exit 0; }
echo '{"error": {"code": "usage", "message": "unknown command"}}' >&2; exit 2"#,
        );
        let e = j.check_compatible().unwrap_err();
        assert_eq!(e.code, CODE_CLI_OLD);
        assert!(e.message.contains("v0.0.1"), "{}", e.message);
        // `jingle schema` without sound_types (no `jingle preview` yet).
        let (_d, j) = fake(r#"echo '{"schema_version": 1, "version": "v0.0.9"}'"#);
        let e = j.check_compatible().unwrap_err();
        assert_eq!(e.code, CODE_CLI_OLD);
        assert!(e.message.contains("v0.0.9"), "{}", e.message);
        // A newer schema than the app knows.
        let (_d, j) = fake(r#"echo '{"schema_version": 2, "version": "v9.0.0", "sound_types": []}'"#);
        assert_eq!(j.check_compatible().unwrap_err().code, CODE_APP_OLD);
        // A jingle that can't run at all keeps its own error.
        let (_d, j) = fake("echo not json");
        assert_eq!(j.check_compatible().unwrap_err().code, CODE_BAD_OUTPUT);
    }

    #[test]
    fn locate_order() {
        let a = tempfile::tempdir().unwrap();
        let b = tempfile::tempdir().unwrap();
        let in_a = script(a.path(), "jingle", "true");
        let in_b = script(b.path(), "jingle", "true");
        let base = OsString::from("/usr/bin:/bin");
        let cands = [PathBuf::from("/nonexistent"), b.path().to_path_buf()];
        let shell_found = || Some(ShellLookup { bin: Some(in_a.clone()), path: "/shell/bin".into() });

        // JINGLE_BIN wins, and a bad one is an error, not a fallback.
        let j = locate_with(Some(in_b.clone().into()), shell_found, &cands, base.clone()).unwrap();
        assert_eq!(j.bin, in_b);
        let e = locate_with(Some("/no/such".into()), shell_found, &cands, base.clone()).unwrap_err();
        assert_eq!(e.code, CODE_NOT_FOUND);

        // Then the login shell, whose PATH is used, plus jingle's dir.
        let j = locate_with(None, shell_found, &cands, base.clone()).unwrap();
        assert_eq!(j.bin, in_a);
        let dirs: Vec<_> = env::split_paths(&j.path_env).collect();
        assert_eq!(dirs, vec![PathBuf::from("/shell/bin"), a.path().to_path_buf()]);

        // Then the candidate dirs, with the app's own PATH.
        let none = || Some(ShellLookup { bin: None, path: "/shell/bin".into() });
        assert_eq!(locate_with(None, none, &cands, base.clone()).unwrap().bin, in_b);
        let j = locate_with(None, || None, &cands, base.clone()).unwrap();
        assert!(env::split_paths(&j.path_env).any(|d| d == Path::new("/bin")));

        let e = locate_with(None, || None, &[PathBuf::from("/nonexistent")], base).unwrap_err();
        assert_eq!(e.code, CODE_NOT_FOUND);
    }

    #[test]
    fn shell_lookup_ignores_rc_noise() {
        let out = b"Welcome!\nsome motd\n\n__JINGLE_BIN__=/opt/homebrew/bin/jingle\n__JINGLE_PATH__=/opt/homebrew/bin:/usr/bin\n";
        let l = parse_shell_lookup(out).unwrap();
        assert_eq!(l.bin, Some(PathBuf::from("/opt/homebrew/bin/jingle")));
        assert_eq!(l.path, OsString::from("/opt/homebrew/bin:/usr/bin"));
        // "jingle not found" leaves bin empty; an alias isn't a path.
        let l = parse_shell_lookup(b"__JINGLE_BIN__=\n__JINGLE_PATH__=/usr/bin\n").unwrap();
        assert_eq!(l.bin, None);
        let l = parse_shell_lookup(b"__JINGLE_BIN__=alias jingle=foo\n__JINGLE_PATH__=/usr/bin\n").unwrap();
        assert_eq!(l.bin, None);
        assert!(parse_shell_lookup(b"garbage").is_none());
    }
}
