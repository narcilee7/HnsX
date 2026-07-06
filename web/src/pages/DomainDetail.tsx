import { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { api, InstanceInfo } from '../api';

export default function DomainDetail() {
  const { domainId } = useParams<{ domainId: string }>();
  const [instances, setInstances] = useState<InstanceInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!domainId) return;
    api.listInstances(domainId)
      .then(setInstances)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [domainId]);

  if (loading) return <p>Loading instances...</p>;
  if (error) return <p className="error">{error}</p>;

  return (
    <div>
      <h2>Instances for {domainId}</h2>
      <div className="actions" style={{ marginBottom: '1rem' }}>
        <Link to="/">← Back</Link>
        <Link to={`/traces/${domainId}`}>Traces</Link>
        <Link to={`/metrics/${domainId}`}>Metrics</Link>
      </div>
      {instances.length === 0 ? (
        <p>No instances found.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Instance ID</th>
              <th>Region</th>
              <th>Tags</th>
              <th>Capabilities</th>
            </tr>
          </thead>
          <tbody>
            {instances.map((i) => (
              <tr key={i.instance_id}>
                <td>{i.instance_id}</td>
                <td>{i.region}</td>
                <td>{i.tags.join(', ')}</td>
                <td>{i.capabilities.join(', ')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
