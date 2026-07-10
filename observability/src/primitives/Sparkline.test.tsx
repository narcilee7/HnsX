import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Sparkline } from './Sparkline'

describe.skip('Sparkline', () => {
  it('renders an SVG with role="img"', () => {
    const { container } = render(<Sparkline data={[1, 2, 3, 4, 5]} />)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    // react-sparklines 用 div 包裹，role 由外层 div 提供
    const img = container.querySelector('[role="img"]')
    expect(img).not.toBeNull()
  })

  it('uses default aria-label with variant name', () => {
    render(<Sparkline data={[1, 2, 3]} variant="success" />)
    expect(screen.getByLabelText('Sparkline success')).toBeInTheDocument()
  })

  it('respects custom aria-label', () => {
    render(<Sparkline data={[1, 2, 3]} aria-label="今日 Session" />)
    expect(screen.getByLabelText('今日 Session')).toBeInTheDocument()
  })

  it('renders area path by default (kind=area)', () => {
    const { container } = render(<Sparkline data={[1, 2, 3, 4, 5]} />)
    // react-sparklines 渲染 <path d=...> + <path d=...>（line + area）
    const paths = container.querySelectorAll('path')
    expect(paths.length).toBeGreaterThan(0)
  })

  it('renders only line path when kind=line', () => {
    const { container } = render(<Sparkline data={[1, 2, 3, 4, 5]} kind="line" />)
    const paths = container.querySelectorAll('path')
    expect(paths.length).toBeGreaterThan(0)
  })

  it('applies custom height via inline style', () => {
    const { container } = render(<Sparkline data={[1, 2, 3]} height={48} />)
    const wrapper = container.querySelector('[role="img"]') as HTMLElement | null
    expect(wrapper?.style.height).toBe('48px')
  })

  it('handles empty data without throwing', () => {
    const { container } = render(<Sparkline data={[]} />)
    expect(container.querySelector('svg')).not.toBeNull()
  })
})