import { useEffect, useState } from "react";
import App from "./App";
import { OpsConsole } from "./OpsConsole";
import { ArchiveConsole } from "./ArchiveConsole";
import { UpdatePrompt } from "./UpdatePrompt";
import "./workspace-navigation.css";

type Route = "workbench" | "infra" | "infra/resources" | "archive";
function route(): Route {
  const value =
    window.location.hash.slice(1) || localStorage.getItem("devopspad-workspace");
  if (value === "infra/resources" || value === "archive") return value;
  return value === "infra" || value?.startsWith("apps") ? "infra" : "workbench";
}
export default function Root() {
  const [current, setCurrent] = useState(route);
  const [visited, setVisited] = useState(current !== "infra" && current !== "archive");
  const [resourceMode, setResourceMode] = useState<"workbench" | "infra">(
    current === "infra/resources" ? "infra" : "workbench",
  );
  useEffect(() => {
    const change = () => {
      const next = route();
      if (window.location.hash !== `#${next}`)
        window.history.replaceState(null, "", `#${next}`);
      setCurrent(next);
      localStorage.setItem("devopspad-workspace", next);
      if (next !== "infra" && next !== "archive") {
        setVisited(true);
        setResourceMode(next === "infra/resources" ? "infra" : "workbench");
      }
    };
    change();
    window.addEventListener("hashchange", change);
    return () => window.removeEventListener("hashchange", change);
  }, []);
  return (
    <>
      {visited && (
        <div style={{ display: current === "infra" || current === "archive" ? "none" : "contents" }}>
          <App mode={resourceMode} />
        </div>
      )}
      {current === "infra" && (
        <OpsConsole
          onWorkbench={() => {
            window.location.hash = "workbench";
          }}
        />
      )}
      {current === "archive" && <ArchiveConsole />}
      <UpdatePrompt />
    </>
  );
}
