import { Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import Domains from './pages/Domains';
import DomainDetail from './pages/DomainDetail';
import Traces from './pages/Traces';
import Metrics from './pages/Metrics';

function App() {
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        <Route index element={<Domains />} />
        <Route path="domains/:domainId" element={<DomainDetail />} />
        <Route path="traces/:domainId" element={<Traces />} />
        <Route path="metrics/:domainId" element={<Metrics />} />
      </Route>
    </Routes>
  );
}

export default App;
