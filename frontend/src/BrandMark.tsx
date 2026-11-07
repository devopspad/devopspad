// DevOpsPad "dp" monogram: white d (dev) and green p (ops) on one stem. Source: devopspad-console/brand/glyph.svg.
export function BrandMark({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="8 8 48 48" aria-hidden="true" fill="none" strokeWidth={5.5} strokeLinecap="round">
      <path d="M32 11V32" stroke="#f1f7f2" />
      <circle cx="22.25" cy="32" r="9.75" stroke="#f1f7f2" />
      <path d="M32 32V53" stroke="#7fd1a4" />
      <circle cx="41.75" cy="32" r="9.75" stroke="#7fd1a4" />
    </svg>
  );
}
