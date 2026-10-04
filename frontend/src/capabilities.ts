// Shared with the Go adapter layer; the UI never advertises unsupported actions.
import catalog from "../../internal/workspace/capabilities.json";
import type { Kind } from "./types";
export const capabilities = catalog;
export function engineCapabilities(kind: Kind) {
  return catalog[kind];
}
