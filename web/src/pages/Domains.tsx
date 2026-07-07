import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, Domain } from '../api';

export default function Domains() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    api.listDomains()
      .then(setDomains)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <p>Loading domains...</p>;
  if (error) return <p className="error">{error}</p>;

  return (
    <div>
      <h2>Domains</h2>
      {domains.length === 0 ? (
        <p>No domains registered yet.</p>
      ) : (
        domains.map((d) => (
          <div key={d.id} className="card">
            <h3>{d.id}</h3>
            <p>Version: {d.version}</p>
            <div className="actions">
              <Link to={`/domains/${d.id}`}>Instances</Link>
              <Link to={`/traces/${d.id}`}>Traces</Link>
              <Link to={`/metrics/${d.id}`}>Metrics</Link>
            </div>
          </div>
        ))
      )}
    </div>
  );
}
