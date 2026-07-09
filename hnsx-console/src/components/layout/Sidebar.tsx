import { Link, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Globe,
  PlayCircle,
  Search,
  ClipboardList,
  CheckSquare,
  Settings,
  Activity,
  Shield,
  Menu,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import { useSettingsStore } from '@/stores/settingsStore'

const navItems = [
  { label: 'Dashboard', href: '/', icon: LayoutDashboard },
  { label: 'Domains', href: '/domains', icon: Globe },
  { label: 'Sessions', href: '/sessions', icon: PlayCircle },
  { label: 'Traces', href: '/traces', icon: Search },
  { label: 'Evals', href: '/evals', icon: CheckSquare },
  { label: 'Observability', href: '/observability', icon: Activity },
  { label: 'Audit', href: '/audit', icon: Shield },
  { label: 'Approvals', href: '/approvals', icon: ClipboardList },
  { label: 'Settings', href: '/settings', icon: Settings },
]

function NavItem({
  href,
  icon: Icon,
  label,
  collapsed,
}: {
  href: string
  icon: React.ElementType
  label: string
  collapsed: boolean
}) {
  const location = useLocation()
  const active = location.pathname === href || location.pathname.startsWith(`${href}/`)

  const linkClass = cn(
    buttonVariants({ variant: active ? 'secondary' : 'ghost' }),
    'w-full justify-start gap-2',
    collapsed && 'justify-center px-0',
  )

  const content = (
    <Link to={href} className={linkClass}>
      <Icon className="h-4 w-4" />
      {!collapsed && <span className="truncate">{label}</span>}
    </Link>
  )

  if (collapsed) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger>{content}</TooltipTrigger>
          <TooltipContent side="right">{label}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
  }

  return content
}

function SidebarContent() {
  const collapsed = useSettingsStore((state) => state.sidebarCollapsed)
  return (
    <div className="flex h-full flex-col gap-2 p-3">
      <div className={cn('flex items-center py-2', collapsed ? 'justify-center' : 'px-2')}>
        <Link to="/" className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground font-bold">
            H
          </div>
          {!collapsed && <span className="text-lg font-semibold">HnsX</span>}
        </Link>
      </div>
      <nav className="flex flex-1 flex-col gap-1">
        {navItems.map((item) => (
          <NavItem key={item.href} {...item} collapsed={collapsed} />
        ))}
      </nav>
    </div>
  )
}

export function Sidebar() {
  return (
    <aside className="hidden h-screen w-60 border-r bg-card lg:block">
      <SidebarContent />
    </aside>
  )
}

export function MobileSidebar() {
  return (
    <Sheet>
      <SheetTrigger render={
        <Button variant="ghost" size="icon" className="lg:hidden">
          <Menu className="h-5 w-5" />
          <span className="sr-only">Open menu</span>
        </Button>
      } />
      <SheetContent side="left" className="w-60 p-0">
        <SidebarContent />
      </SheetContent>
    </Sheet>
  )
}
