import Badge from "./Badge";

// Header row for padding-0 cards (title + optional note badge).
export default function SectionHead({ title, note }: { title: string; note?: string }) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "18px 22px" }}>
      <span style={{ fontSize: 16, fontWeight: 700 }}>{title}</span>
      {note ? (
        <Badge tone="neutral" variant="outline">
          {note}
        </Badge>
      ) : null}
    </div>
  );
}
