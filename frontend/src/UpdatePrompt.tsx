import { useEffect, useState } from "react";
import { Download, X } from "lucide-react";
import { api, desktop, errorText } from "./api";
import type { UpdateInfo } from "./types";

const skippedKey = "devopspad-update-skipped";
const recheckMs = 6 * 60 * 60 * 1000;

// Checks GitHub for a newer release at startup and every few hours. Failures stay
// silent: an offline machine or a rate-limited API must not interrupt work.
export function UpdatePrompt() {
  const [update, setUpdate] = useState<UpdateInfo>();
  const [hidden, setHidden] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    if (!desktop) return;
    const check = () =>
      api
        .CheckForUpdate()
        .then((info) => {
          if (
            info.available &&
            localStorage.getItem(skippedKey) !== info.latest
          )
            setUpdate(info);
        })
        .catch(() => {});
    void check();
    const timer = window.setInterval(check, recheckMs);
    return () => window.clearInterval(timer);
  }, []);
  if (!update || hidden === update.latest) return null;
  const open = (url: string) =>
    void api.OpenUpdate(url).catch((e) => setError(errorText(e)));
  return (
    <div className="update-prompt" role="dialog" aria-label="DevOpsPad update">
      <div className="update-prompt-head">
        <Download size={16} />
        <strong>DevOpsPad v{update.latest} is available</strong>
        <button
          className="update-prompt-close"
          aria-label="Remind me later"
          onClick={() => setHidden(update.latest)}
        >
          <X size={14} />
        </button>
      </div>
      <p>
        You have v{update.current}. Open the downloaded DMG and drag DevOpsPad
        into Applications to replace it. Connections and project settings are kept.
        <button className="update-prompt-link" onClick={() => open(update.url)}>
          Release notes
        </button>
      </p>
      {error && (
        <p role="alert" className="update-prompt-error">
          {error}
        </p>
      )}
      <div className="update-prompt-actions">
        <button
          className="update-prompt-secondary"
          onClick={() => {
            localStorage.setItem(skippedKey, update.latest);
            setUpdate(undefined);
          }}
        >
          Skip this version
        </button>
        <button
          className="update-prompt-secondary"
          onClick={() => setHidden(update.latest)}
        >
          Remind me later
        </button>
        <button
          className="update-prompt-primary"
          onClick={() => open(update.downloadUrl || update.url)}
        >
          Download update
        </button>
      </div>
    </div>
  );
}
