import { DEFAULT_BRAND_NAME, brandInitials, panelBrand } from '@/lib/brand/brand';

import BrandGlyph from './BrandGlyph';
import './BrandMark.css';

export interface BrandMarkProps {
  compact?: boolean;
  className?: string;
}

export default function BrandMark({ compact = false, className }: BrandMarkProps) {
  const { name, logo } = panelBrand();
  const label = name || DEFAULT_BRAND_NAME;
  const customName = !!name && name !== DEFAULT_BRAND_NAME;
  const showInitials = compact && !logo && customName;
  return (
    <span className={`brand-mark${className ? ` ${className}` : ''}`} title={label}>
      {logo && <img className="brand-mark-logo" src={logo} alt="" />}
      {!logo && !showInitials && <BrandGlyph className="brand-mark-logo" />}
      {!compact && (
        <span className="brand-mark-name" dir="auto">
          {label}
        </span>
      )}
      {showInitials && (
        <span className="brand-mark-name" dir="auto">
          {brandInitials(name)}
        </span>
      )}
    </span>
  );
}
