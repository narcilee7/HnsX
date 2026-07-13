import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ErrorState } from './Error'

describe('ErrorState', () => {
  it('renders default title and description', () => {
    render(<ErrorState description="Something failed" />)
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(screen.getByText('Something failed')).toBeInTheDocument()
  })

  it('renders custom title', () => {
    render(<ErrorState title="Custom Error" description="Details" />)
    expect(screen.getByText('Custom Error')).toBeInTheDocument()
  })

  it('calls onRetry when retry button clicked', async () => {
    const onRetry = vi.fn()
    render(<ErrorState title="Oops" description="Retry me" onRetry={onRetry} />)

    const button = screen.getByRole('button', { name: /retry/i })
    await userEvent.click(button)
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('does not render retry button when onRetry is omitted', () => {
    render(<ErrorState title="Oops" description="No retry" />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
