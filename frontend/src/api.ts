import { Events } from "@wailsio/runtime";
import * as AppService from "../bindings/my-local-name/appservice";
import type { AppState, Diagnostics, InterfaceOption, LogLine } from "../bindings/my-local-name/models";

export type { AppState, Diagnostics, InterfaceOption, LogLine };

export const api = AppService;

export function onState(callback: (state: AppState) => void): () => void {
  return Events.On("state:update", (event) => callback(event.data));
}

export function onLog(callback: (line: LogLine) => void): () => void {
  return Events.On("log:line", (event) => callback(event.data));
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message.replace(/^Error:\s*/, "");
  }
  if (typeof err === "string") {
    return err;
  }
  return "操作失败，请稍后重试";
}

export function isActive(status: string | undefined): boolean {
  return status === "probing" || status === "announcing" || status === "broadcasting";
}
