import Link from "next/link";
import { notFound } from "next/navigation";
import { Clock, MapPin, Pencil, Plus } from "lucide-react";
import { getSession } from "@/lib/services/sessions";
import { PageHeader } from "@/components/ui/PageHeader";
import { Card } from "@/components/ui/Card";
import { Badge, type BadgeTone } from "@/components/ui/Badge";
import { buttonClasses } from "@/components/ui/Button";
import { AttendanceControls } from "@/components/schedule/AttendanceControls";
import { SessionControls } from "@/components/schedule/SessionControls";
import { formatDate, formatMoney, formatTime } from "@/lib/utils";

const STATUS_TONE: Record<string, BadgeTone> = {
  scheduled: "neutral",
  completed: "paid",
  cancelled: "overdue",
};

export default async function SessionDetail({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const session = await getSession(id);
  if (!session) notFound();

  const attended = session.attendees.filter((a) => a.status === "attended").length;
  const isPrivate = session.type === "private";
  const lead = session.attendees[0];

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow={formatDate(session.start)}
        title={session.title}
        action={
          <Link href={`/schedule/${id}/edit`} className={buttonClasses("ghost", "sm")}>
            <Pencil className="h-4 w-4" />
            Edit
          </Link>
        }
      />

      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={session.type === "group" ? "bronze" : "info"}>
          {session.type === "group" ? "Group class" : "Private"}
        </Badge>
        <Badge tone={STATUS_TONE[session.status]}>
          {session.status[0].toUpperCase() + session.status.slice(1)}
        </Badge>
      </div>

      <Card className="space-y-2.5 p-4">
        <div className="flex items-center gap-2.5 text-sm">
          <Clock className="h-4 w-4 shrink-0 text-faint" />
          <span>
            {formatDate(session.start)} · {formatTime(session.start)}
            <span className="text-faint"> · {session.durationMin} min</span>
          </span>
        </div>
        {session.location ? (
          <div className="flex items-center gap-2.5 text-sm">
            <MapPin className="h-4 w-4 shrink-0 text-faint" />
            <span>{session.location}</span>
          </div>
        ) : null}
        {isPrivate && session.fee ? (
          <div className="flex items-center justify-between border-t border-line pt-2.5 text-sm">
            <span className="text-faint">Session fee</span>
            <span className="font-display tnum">{formatMoney(session.fee)}</span>
          </div>
        ) : null}
      </Card>

      <section className="space-y-2">
        <div className="flex items-center justify-between px-1">
          <h2 className="label-eyebrow text-[0.625rem] text-faint">
            {isPrivate ? "Trainee" : `Attendance · ${attended}/${session.attendees.length} here`}
          </h2>
          {isPrivate && lead ? (
            <Link
              href={`/payments/new?trainee=${lead.traineeId}&type=private&session=${id}${
                session.fee ? `&amount=${session.fee}` : ""
              }`}
              className="inline-flex items-center gap-1 text-sm font-medium text-bronze"
            >
              <Plus className="h-3.5 w-3.5" />
              Log payment
            </Link>
          ) : null}
        </div>
        <AttendanceControls sessionId={id} attendees={session.attendees} />
      </section>

      {session.notes ? (
        <Card className="p-4">
          <p className="label-eyebrow mb-1 text-[0.625rem] text-faint">Notes</p>
          <p className="whitespace-pre-wrap text-sm text-muted">{session.notes}</p>
        </Card>
      ) : null}

      <div className="border-t border-line pt-4">
        <SessionControls id={id} status={session.status} seriesId={session.seriesId} />
      </div>
    </div>
  );
}
