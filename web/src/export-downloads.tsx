import { Icon } from './components/ui/icon'
export function ExportDownloads({ eventID, audience }: { eventID: string; audience: 'organiser' | 'mentor' | 'participant' }) {
  const base = `/api/v1/events/${eventID}/exports`
  return <section className="fleet-planning-section" aria-label={`${audience} downloads`}><h3>Download your {audience} snapshot</h3><p>Downloads use your current permissions. The run sheet works offline and can be printed. Keep downloaded snapshots private.</p><div className="fleet-pattern-actions"><a href={`${base}/readiness.csv?audience=${audience}`} download><Icon name="download"/> Readiness CSV</a><a href={`${base}/run-sheet.html?audience=${audience}`} download><Icon name="download"/> Printable run sheet</a></div></section>
}
