import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MetricCard } from './MetricCard'

describe('MetricCard', () => {
  it('renders label, value, and unit', () => {
    render(<MetricCard label="今日 Session" value={1247} unit="次" />)
    expect(screen.getByText('今日 Session')).toBeInTheDocument()
    expect(screen.getByText('1,247')).toBeInTheDocument()
    expect(screen.getByText('次')).toBeInTheDocument()
  })

  it('applies formatValue for numeric values', () => {
    render(<MetricCard label="成本" value={0.42} formatValue={(n) => `$${n.toFixed(2)}`} unit="USD" />)
    expect(screen.getByText('$0.42')).toBeInTheDocument()
  })

  it('renders string value verbatim', () => {
    render(<MetricCard label="Status" value="Online" />)
    expect(screen.getByText('Online')).toBeInTheDocument()
  })

  it('renders ReactNode value (e.g., custom JSX)', () => {
    render(<MetricCard label="State" value={<span data-testid="custom">Custom Node</span>} />)
    expect(screen.getByTestId('custom')).toBeInTheDocument()
  })

  it('renders caption when provided', () => {
    render(<MetricCard label="Label" value={1} caption="对比昨日" />)
    expect(screen.getByText('对比昨日')).toBeInTheDocument()
  })

  it('invokes onClick when card is clickable', async () => {
    const onClick = vi.fn()
    render(<MetricCard label="X" value={1} onClick={onClick} />)
    await userEvent.click(screen.getByText('X'))
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('does NOT throw when clicked without onClick', async () => {
    render(<MetricCard label="X" value={1} />)
    await userEvent.click(screen.getByText('X'))
    // 仅断言不抛错
  })

  it('renders trend slot content', () => {
    render(
      <MetricCard
        label="X"
        value={1}
        trend={<span data-testid="trend">+5%</span>}
      />,
    )
    expect(screen.getByTestId('trend')).toBeInTheDocument()
  })

  it('renders footer slot content', () => {
    render(
      <MetricCard
        label="X"
        value={1}
        footer={<div data-testid="footer">footer</div>}
      />,
    )
    expect(screen.getByTestId('footer')).toBeInTheDocument()
  })
})