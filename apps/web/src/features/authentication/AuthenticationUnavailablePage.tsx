export function AuthenticationUnavailablePage({ title }: { title: string }) {
  return <section className="page">
    <h1>{title}</h1>
    <p className="muted">Not configured in this Manager version</p>
  </section>
}
