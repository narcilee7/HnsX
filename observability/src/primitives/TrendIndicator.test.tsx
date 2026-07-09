import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TrendIndicator } from './TrendIndicator'

describe('TrendIndicator', () => {
  it('renders positive delta with default percent format', () => {
    render(<TrendIndicator value={120} previous={100} />)
    expect(screen.getByText('+20.0%')).toBeInTheDocument()
  })

  it('renders negative delta', () => {
    render(<TrendIndicator value={80} previous={100} />)
    expect(screen.getByText('-20.0%')).toBeInTheDocument()
  })

  it('renders flat delta as "0.0%"', () => {
    render(<TrendIndicator value={100} previous={100} />)
    expect(screen.getByText('+0.0%')).toBeInTheDocument()
  })

  it('uses explicit delta prop over previous', () => {
    render(<TrendIndicator value={50} previous={999} delta={0.05} />)
    expect(screen.getByText('+0.1%')).toBeInTheDocument()
  })

  it('uses custom format function', () => {
    render(
      <TrendIndicator
        value={150}
        previous={100}
        format={(n) => `${n > 0 ? '↑' : '↓'} ${Math.abs(n).toFixed(0)}pp`}
      />,
    )
    expect(screen.getByText(/↑ 50pp/)).toBeInTheDocument()
  })

  it('flips good/bad with goodWhen="down"', () => {
    // 增长 20% — 但 goodWhen=down 所以应该是 bad（红色调）
    const { container } = render(<TrendIndicator value={120} previous={100} goodWhen="down" />)
    // 找到含 +20% 的 span
    const span = screen.getByText('+20.0%')
    expect(span).toBeInTheDocument()
    // 验证它有 danger-text 颜色类（不在 good class 里）
    expect(container.innerHTML).toMatch(/danger-text|danger-soft/)
  })

  it('invertColor swaps good/bad judgment', () => {
    // 下降 50% — 默认 goodWhen=up 所以是 bad；invertColor=true 后变 good
    const { container: c1 } = render(<TrendIndicator value={50} previous={100} />)
    expect(c1.innerHTML).toMatch(/danger-text|danger-soft/)

    const { container: c2 } = render(<TrendIndicator value={50} previous={100} invertColor />)
    expect(c2.innerHTML).toMatch(/success-text|success-soft/)
  })

  it('handles zero previous without throwing', () => {
    render(<TrendIndicator value={100} previous={0} />)
    // 应当 fallback 到 0%
    expect(screen.getByText('+0.0%')).toBeInTheDocument()
  })
})