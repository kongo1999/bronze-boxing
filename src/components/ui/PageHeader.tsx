export function PageHeader({
  title,
  eyebrow,
  action,
}: {
  title: string;
  eyebrow?: string;
  action?: React.ReactNode;
}) {
  return (
    <header className="flex items-end justify-between gap-3 pb-1">
      <div className="min-w-0">
        {eyebrow ? (
          <p className="label-eyebrow text-[0.625rem] text-bronze">{eyebrow}</p>
        ) : null}
        <h1 className="truncate font-display text-2xl font-semibold tracking-tight">
          {title}
        </h1>
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </header>
  );
}
