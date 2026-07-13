import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ErrorBoundary } from './ErrorBoundary'

const ThrowError = ({ message }: { message: string }) => {
  throw new Error(message)
}

describe('ErrorBoundary', () => {
  it('renders children when no error', () => {
    render(
      <ErrorBoundary>
        <div>Healthy content</div>
      </ErrorBoundary>,
    )
    expect(screen.getByText('Healthy content')).toBeInTheDocument()
  })

  it('renders error UI when child throws', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary>
        <ThrowError message="Boom" />
      </ErrorBoundary>,
    )

    expect(screen.getByText('页面出现错误')).toBeInTheDocument()
    expect(screen.getByText('Boom')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /刷新页面/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /返回首页/i })).toBeInTheDocument()
  })

  it('uses custom fallback when provided', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary fallback={() => <div>Custom fallback</div>}>
        <ThrowError message="Boom" />
      </ErrorBoundary>,
    )

    expect(screen.getByText('Custom fallback')).toBeInTheDocument()
  })
})
