import { DEFAULT_BRAND_NAME, brandInitials, panelBrand } from '@/lib/brand/brand';

import './BrandMark.css';

export interface BrandMarkProps {
  compact?: boolean;
  className?: string;
}

export default function BrandMark({ compact = false, className }: BrandMarkProps) {
  const { name, logo } = panelBrand();
  const label = name || DEFAULT_BRAND_NAME;
  return (
    <span className={`brand-mark${className ? ` ${className}` : ''}`} title={label}>
      {logo && <img className="brand-mark-logo" src={logo} alt="" />}
      {!(compact && logo) && (
        <span className="brand-mark-name" dir="auto">
          {compact ? brandInitials(name) : label}
        </span>
      )}
    </span>
  );
}
