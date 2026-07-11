import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuthStore } from '@/stores/authStore'

export default function LoginPage() {
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const setStoreToken = useAuthStore((state) => state.setToken)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (!token.trim()) {
      setError('Token is required')
      return
    }
    setStoreToken(token.trim())
    navigate('/')
  }

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)] items-center justify-center">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Sign in to HnsX</CardTitle>
          <CardDescription>
            Paste the bearer token issued by your operator or SSO provider.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="token">Bearer token</Label>
              <Input
                id="token"
                type="password"
                placeholder="eyJhbG..."
                value={token}
                onChange={(e) => setToken(e.target.value)}
              />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" className="w-full">
              Sign in
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
