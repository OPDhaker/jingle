// Typed wrappers over the Rust commands (gui/src-tauri/src/commands.rs).
// Every one of them runs the jingle CLI with --json; the types mirror
// docs/json.md. Fields jingle may add later are simply ignored here.

import { invoke } from "@tauri-apps/api/core";

export interface JingleError {
  code: string;
  message: string;
  exit: number;
}

export interface Problem {
  code: string;
  severity: "error" | "warning";
  message: string;
  fix?: string;
}

export interface Player {
  name: string;
  path: string;
  volume: boolean;
}

/** `config list`'s values: every key, typed by key. */
export type ConfigValues = Record<string, boolean | number | string | string[]>;

export interface Status {
  installed: boolean;
  hooks_dir: string;
  global_hooks_path: string | null;
  previous_hooks_path: string | null;
  config_path: string;
  config: ConfigValues;
  player: Player | null;
  git: { path: string; version: string } | null;
  problems: Problem[];
}

export interface Snapshot {
  jingle: { bin: string } | null;
  status: Status | null;
  error: JingleError | null;
  headline: string;
}

export interface ConfigKey {
  key: string;
  type: "bool" | "int" | "string" | "list";
  doc: string;
  values?: string[];
  min?: number;
  max?: number;
}

export interface Schema {
  schema_version: number;
  version: string;
  events: string[];
  config_keys: ConfigKey[];
  builtin_sounds: { name: string; doc: string; default_for: string[] }[];
  /** Missing from jingle builds older than the GUI. */
  sound_types?: string[];
}

export interface Version {
  version: string;
  schema_version: number;
}

export interface RepoReport extends Status {
  repo: { path: string; hooks_path: string | null; hooks_path_scope?: string; husky: boolean };
}

export const api = {
  snapshot: () => invoke<Snapshot>("snapshot"),
  relocate: () => invoke<Snapshot>("relocate"),
  schema: () => invoke<Schema>("schema"),
  version: () => invoke<Version>("version"),
  configSet: (key: string, values: string[]) => invoke("config_set", { key, values }),
  configAdd: (key: string, sounds: string[]) => invoke("config_add", { key, sounds }),
  configRemove: (key: string, sounds: string[]) => invoke("config_remove", { key, sounds }),
  play: (event: string, sound?: string) => invoke("play", { event, sound: sound ?? null }),
  preview: (sound: string) => invoke("preview", { sound }),
  install: () => invoke("install"),
  uninstall: () => invoke("uninstall"),
  doctorRepo: (dir: string) => invoke<RepoReport>("doctor_repo", { dir }),
};

/** The message to show for anything a command threw. */
export function errorMessage(e: unknown): string {
  if (e && typeof e === "object" && "message" in e && typeof e.message === "string") return e.message;
  return String(e);
}
