import { Database, Server } from "lucide-react";
export type Workspace = "workbench" | "infra";
export function WorkspaceNavigation({
  active,
  disabled = false,
}: {
  active: Workspace;
  disabled?: boolean;
}) {
  return (
    <div className="workspace-navigation" role="group" aria-label="工作区导航">
      {(
        [
          { id: "workbench", label: "中间件", icon: Database },
          { id: "infra", label: "Infra", icon: Server },
        ] as const
      ).map(({ id, label, icon: Icon }) => (
        <button
          key={id}
          className={active === id ? "selected" : ""}
          aria-current={active === id ? "page" : undefined}
          disabled={disabled}
          onClick={() => {
            window.location.hash = id;
          }}
        >
          <Icon size={20} />
          <span>{label}</span>
        </button>
      ))}
    </div>
  );
}
