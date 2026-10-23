import { ArrowDownAZ, ArrowUpAZ, BookOpen, CalendarDays, Code2, Columns3, Filter, GitBranch, LayoutDashboard, List, Palette, Type, UsersRound, ArrowDownToLine, ArrowLeft, ArrowRight, ArrowUpRight, Check, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, CircleAlert, CircleHelp, Info, LoaderCircle, LogOut, MoreHorizontal, Plus, RefreshCw, Save, Search, Send, ShieldCheck, Trash2, Upload, UserRound, X } from 'lucide-react'
import { cn } from '@/lib/utils'

const icons = { book: BookOpen, calendar: CalendarDays, code: Code2, columns: Columns3, filter: Filter, layout: LayoutDashboard, list: List, palette: Palette, repository: GitBranch, sortAscending: ArrowDownAZ, sortDescending: ArrowUpAZ, team: UsersRound, type: Type, add: Plus, back: ArrowLeft, check: Check, close: X, down: ChevronDown, download: ArrowDownToLine, external: ArrowUpRight, help: CircleHelp, info: Info, logout: LogOut, more: MoreHorizontal, next: ArrowRight, nextPage: ChevronRight, pending: LoaderCircle, previousPage: ChevronLeft, refresh: RefreshCw, remove: Trash2, save: Save, search: Search, send: Send, shield: ShieldCheck, success: CheckCircle2, upload: Upload, user: UserRound, warning: CircleAlert }
export type IconName = keyof typeof icons
export const iconNames: readonly IconName[] = Object.freeze(Object.keys(icons) as IconName[])
// Icons supplement a control's accessible name or visible status text. SVGs never
// announce duplicate labels and cannot become keyboard targets.
export function Icon({ name, className }: { name: IconName; className?: string }) {
  const Glyph = icons[name]
  return <Glyph aria-hidden="true" focusable="false" strokeWidth={1.75} className={cn('fleet-icon', className)} />
}
