import { Link, Outlet } from 'react-router-dom';

export default function Layout() {
  return (
    <div className="layout">
      <header>
        <h1>HnsX Console</h1>
        <nav>
          <Link to="/">Domains</Link>
        </nav>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  );
}
