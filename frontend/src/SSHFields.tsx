import { FolderOpen, LockKeyhole } from "lucide-react";
import { api, desktop, errorText } from "./api";
import type { SSHConfig } from "./types";

const defaults: SSHConfig = {
  enabled: false,
  host: "",
  port: 22,
  username: "",
  authMethod: "key",
  privateKeyPath: "",
  knownHostsPath: "",
  hostKeyFingerprint: "",
  hasPassword: false,
  hasPassphrase: false,
};

export function SSHFields({
  value,
  onChange,
  onError,
  cluster,
}: {
  value?: SSHConfig;
  onChange: (value: SSHConfig) => void;
  onError: (error: string) => void;
  cluster: boolean;
}) {
  const ssh = { ...defaults, ...value };
  const patch = (v: Partial<SSHConfig>) => onChange({ ...ssh, ...v });
  const chooseFile = async (kind: "privateKey" | "knownHosts") => {
    try {
      const path = await api.SelectSSHFile(kind);
      if (path)
        patch(
          kind === "privateKey"
            ? { privateKeyPath: path }
            : { knownHostsPath: path },
        );
    } catch (e) {
      onError(errorText(e));
    }
  };
  return (
    <section className="ssh-settings" aria-label="SSH tunnel settings">
      <label className="check-label">
        <input
          type="checkbox"
          checked={ssh.enabled}
          onChange={(e) => patch({ enabled: e.target.checked })}
        />
        <LockKeyhole size={15} /> Connect through SSH tunnel
      </label>
      {ssh.enabled && (
        <div className="ssh-fields">
          <p className="field-help">
            {cluster
              ? "The selected kubeconfig API server is reached from the SSH host. Certificate verification follows your kubeconfig."
              : "Your database host and port are reached from the SSH host. TLS settings still apply."}
          </p>
          <div className="form-grid host-grid">
            <label>
              SSH host
              <input
                required
                placeholder="bastion.example.com"
                value={ssh.host}
                onChange={(e) => patch({ host: e.target.value })}
              />
            </label>
            <label>
              SSH port
              <input
                required
                type="number"
                min="1"
                max="65535"
                value={ssh.port}
                onChange={(e) => patch({ port: Number(e.target.value) })}
              />
            </label>
          </div>
          <div className="form-grid">
            <label>
              SSH username
              <input
                required
                autoComplete="off"
                placeholder="ubuntu"
                value={ssh.username}
                onChange={(e) => patch({ username: e.target.value })}
              />
            </label>
            <label>
              SSH authentication
              <select
                value={ssh.authMethod}
                onChange={(e) =>
                  patch({
                    authMethod: e.target.value as SSHConfig["authMethod"],
                    password: "",
                    passphrase: "",
                  })
                }
              >
                <option value="key">Private key</option>
                <option value="password">Password</option>
                <option value="agent">SSH Agent</option>
              </select>
            </label>
          </div>
          {ssh.authMethod === "password" && (
            <label>
              SSH password
              <input
                type="password"
                autoComplete="new-password"
                value={ssh.password || ""}
                placeholder={
                  ssh.hasPassword
                    ? "Saved in keychain · leave blank to keep"
                    : "SSH password"
                }
                onChange={(e) => patch({ password: e.target.value })}
              />
            </label>
          )}
          {ssh.authMethod === "key" && (
            <>
              <label>
                Private key file
                <div className="file-input">
                  <input
                    required
                    placeholder="~/.ssh/id_ed25519"
                    value={ssh.privateKeyPath}
                    onChange={(e) => patch({ privateKeyPath: e.target.value })}
                  />
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="Choose SSH private key"
                    disabled={!desktop}
                    onClick={() => void chooseFile("privateKey")}
                  >
                    <FolderOpen size={17} />
                  </button>
                </div>
              </label>
              <label>
                Key passphrase
                <input
                  type="password"
                  autoComplete="new-password"
                  value={ssh.passphrase || ""}
                  placeholder={
                    ssh.hasPassphrase
                      ? "Saved in keychain · leave blank to keep"
                      : "Optional for an unencrypted key"
                  }
                  onChange={(e) => patch({ passphrase: e.target.value })}
                />
              </label>
            </>
          )}
          {ssh.authMethod === "agent" && (
            <p className="field-help">
              Uses the SSH Agent available to the desktop app. If no agent is
              available, choose a private key instead.
            </p>
          )}
          <label>
            Known hosts file
            <div className="file-input">
              <input
                placeholder="~/.ssh/known_hosts (default)"
                value={ssh.knownHostsPath}
                disabled={!!ssh.hostKeyFingerprint}
                onChange={(e) => patch({ knownHostsPath: e.target.value })}
              />
              <button
                type="button"
                className="icon-button"
                aria-label="Choose known hosts file"
                disabled={!desktop || !!ssh.hostKeyFingerprint}
                onClick={() => void chooseFile("knownHosts")}
              >
                <FolderOpen size={17} />
              </button>
            </div>
          </label>
          <label>
            Host key fingerprint
            <input
              placeholder="SHA256:… (optional alternative to known_hosts)"
              value={ssh.hostKeyFingerprint}
              onChange={(e) =>
                patch({ hostKeyFingerprint: e.target.value.trim() })
              }
            />
          </label>
          <p className="field-help">
            Only trusted host keys are accepted. Get the fingerprint from your
            server administrator, or use an existing known_hosts entry.
          </p>
        </div>
      )}
    </section>
  );
}
