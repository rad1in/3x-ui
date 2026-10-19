import { useId } from 'react';

export interface BrandGlyphProps {
  size?: number;
  className?: string;
}

export default function BrandGlyph({ size = 28, className }: BrandGlyphProps) {
  const gradientId = `brand-glyph-${useId().replace(/:/g, '')}`;
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 1024 1024"
      aria-hidden="true"
      focusable="false"
    >
      <defs>
        <linearGradient
          id={gradientId}
          x1="0"
          y1="0"
          x2="1024"
          y2="1024"
          gradientUnits="userSpaceOnUse"
        >
          <stop offset="0" stopColor="#8d85f2" />
          <stop offset="0.5" stopColor="#5b4fe0" />
          <stop offset="1" stopColor="#3b30b0" />
        </linearGradient>
      </defs>
      <rect width="1024" height="1024" rx="230" fill={`url(#${gradientId})`} />
      <path
        d="M553 551H263A145 145 0 0 1 553 551M263 551A145 145 0 0 0 408 696H560A97 97 0 0 0 657 599V328M594 459H763"
        fill="none"
        stroke="#fff"
        strokeWidth="62"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
