import { Link, useLocation } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button'
import { Briefcase, FileCode2, PlayCircle, FlaskConical } from 'lucide-react'

interface DomainNavProps {
  domainId: string
}

const tabs = [
  { label: 'Workspace', href: (id: string) => `/domains/${id}/workspace`, icon: Briefcase },
  { label: 'Editor', href: (id: string) => `/domains/${id}`, icon: FileCode2 },
  { label: 'Runs', href: (id: string) => `/sessions?domain=${id}`, icon: PlayCircle },
  { label: 'Evals', href: (_id: string) => `/evals`, icon: FlaskConical },
]

export function DomainNav({ domainId }: DomainNavProps) {
  const location = useLocation()

  return (
    <nav className="flex items-center gap-1 border-b pb-1">
      {tabs.map((tab) => {
        const href = tab.href(domainId)
        const active =
          location.pathname === href ||
          (tab.label === 'Editor'
            ? location.pathname === `/domains/${domainId}`
            : false)
        return (
          <Link
            key={tab.label}
            to={href}
            className={cn(
              buttonVariants({ variant: active ? 'secondary' : 'ghost' }),
              'gap-1.5 text-sm',
            )}
          >
            <tab.icon className="h-4 w-4" />
            {tab.label}
          </Link>
        )
      })}
    </nav>
  )
}
