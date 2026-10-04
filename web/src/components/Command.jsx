import CopyButton from './CopyButton.jsx'

// Commands in ink, comments in muted. Painting the product's main argument in
// the faintest colour on the page was a mistake.
export default function Command({ lines, className = '' }) {
  return (
    <div className={`group relative rounded border border-edge bg-ground ${className}`}>
      <pre className="overflow-x-auto px-3 py-2 font-mono text-xs leading-relaxed">
        {lines.map((line, i) => (
          <div key={i} className={line.startsWith('#') ? 'text-faint' : 'text-ink'}>
            {line || ' '}
          </div>
        ))}
      </pre>
      {/* Visible, if quiet: hidden until hovered it did not exist on a phone. */}
      <CopyButton
        text={lines.join('\n')}
        label={`Copy ${lines.length === 1 ? 'the command' : 'the commands'}`}
        className="absolute right-1.5 top-1.5 opacity-60 hover:opacity-100 focus-visible:opacity-100 group-hover:opacity-100"
      />
    </div>
  )
}
