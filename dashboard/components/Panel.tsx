import type { CSSProperties, ReactNode } from "react";

export default function Panel({
  title,
  note,
  children,
  style,
}: {
  title: string;
  note?: string;
  children: ReactNode;
  style?: CSSProperties;
}) {
  return (
    <section className="panel reveal" style={style}>
      <div className="panel-head">
        <span className="panel-title">{title}</span>
        {note ? <span className="panel-note">{note}</span> : null}
      </div>
      {children}
    </section>
  );
}
