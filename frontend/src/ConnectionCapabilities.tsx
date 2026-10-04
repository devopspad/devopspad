import { CheckCircle2, CircleHelp } from "lucide-react";
import { engineCapabilities } from "./capabilities";
import { engines, type Kind } from "./types";
import { Modal } from "./Modal";
export function ConnectionCapabilities({
  kind,
  onClose,
}: {
  kind: Kind;
  onClose: () => void;
}) {
  const c = engineCapabilities(kind);
  return (
    <Modal
      title={`${engines[kind].name} capabilities`}
      subtitle="Available in this release"
      onClose={onClose}
    >
      <div className="form-body capability-content">
        <div className="capability-summary">
          <strong>{c.language}</strong>
          <span>{c.resourceLabel} · Read-only access</span>
        </div>
        {c.features.map((feature) => (
          <p key={feature}>
            <CheckCircle2 size={16} />
            {feature}
          </p>
        ))}
        <div className="capability-limitations">
          <h3>
            <CircleHelp size={15} />
            Current limitations
          </h3>
          {c.limitations.map((limit) => (
            <p key={limit}>{limit}</p>
          ))}
        </div>
      </div>
    </Modal>
  );
}
