import Link from "next/link";
import { ChevronRight, UserPlus, Users } from "lucide-react";
import { listTrainees } from "@/lib/services/trainees";
import { PageHeader } from "@/components/ui/PageHeader";
import { EmptyState } from "@/components/ui/EmptyState";
import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { buttonClasses } from "@/components/ui/Button";
import { TraineeSearch } from "@/components/trainees/TraineeSearch";
import { formatMoney } from "@/lib/utils";

export default async function TraineesPage({
  searchParams,
}: {
  searchParams: Promise<{ q?: string }>;
}) {
  const { q } = await searchParams;
  const trainees = await listTrainees({ search: q });
  const active = trainees.filter((t) => t.status === "active").length;

  return (
    <div className="space-y-4">
      <PageHeader
        eyebrow="Roster"
        title="Crew"
        action={
          <Link href="/trainees/new" className={buttonClasses("primary", "sm")}>
            <UserPlus className="h-4 w-4" />
            Add
          </Link>
        }
      />
      <TraineeSearch initialQuery={q} />

      {trainees.length === 0 ? (
        <EmptyState
          icon={Users}
          title={q ? "No matches" : "No trainees yet"}
          description={
            q
              ? "Try a different name."
              : "Add your first trainee to start tracking dues and attendance."
          }
          action={
            q ? undefined : (
              <Link href="/trainees/new" className={buttonClasses("primary", "sm")}>
                <UserPlus className="h-4 w-4" />
                Add trainee
              </Link>
            )
          }
        />
      ) : (
        <>
          <p className="px-1 text-sm text-faint">
            {active} active
            {trainees.length !== active ? ` · ${trainees.length - active} inactive` : ""}
          </p>
          <ul className="space-y-2">
            {trainees.map((t) => (
              <li key={t.id}>
                <Link
                  href={`/trainees/${t.id}`}
                  className="flex items-center gap-3 rounded-2xl border border-line bg-surface p-3 transition-colors hover:border-bronze/30"
                >
                  <Avatar name={t.name} className={t.status === "inactive" ? "opacity-50" : ""} />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">{t.name}</p>
                    <p className="truncate text-sm text-muted">
                      {t.phone ??
                        (t.skillLevel
                          ? t.skillLevel[0].toUpperCase() + t.skillLevel.slice(1)
                          : "No phone")}
                    </p>
                  </div>
                  <div className="flex flex-col items-end gap-1">
                    {t.monthlyFee > 0 ? (
                      <span className="font-display text-sm tnum">
                        {formatMoney(t.monthlyFee)}
                        <span className="text-xs text-faint">/mo</span>
                      </span>
                    ) : null}
                    {t.status === "inactive" ? <Badge tone="neutral">Inactive</Badge> : null}
                  </div>
                  <ChevronRight className="h-4 w-4 shrink-0 text-faint" />
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
