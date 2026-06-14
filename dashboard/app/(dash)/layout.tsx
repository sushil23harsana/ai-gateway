import Sidebar from "@/components/Sidebar";

// Shared dashboard shell: persistent sidebar + a content area. Each page renders
// its own Topbar (fixed) followed by a scrollable body.
export default function DashLayout({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw", background: "var(--bg-base)", overflow: "hidden" }}>
      <Sidebar />
      <main
        style={{
          flex: 1,
          minWidth: 0,
          display: "flex",
          flexDirection: "column",
          background: "var(--bg-canvas)",
          overflow: "hidden",
        }}
      >
        {children}
      </main>
    </div>
  );
}
