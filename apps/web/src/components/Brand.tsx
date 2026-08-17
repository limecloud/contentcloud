import type { SVGProps } from 'react';

export function BrandMark({className='', ...props}: SVGProps<SVGSVGElement>) {
  return <svg {...props} className={`brand-logo-mark ${className}`.trim()} viewBox="0 0 30 30" aria-hidden="true" focusable="false">
    <g className="brand-logo-grid">
      <rect x="2" y="2" width="7" height="7" rx="1.25" />
      <rect x="11.5" y="2" width="7" height="7" rx="1.25" />
      <rect x="21" y="2" width="7" height="7" rx="1.25" />
      <rect x="2" y="11.5" width="7" height="7" rx="1.25" />
      <rect x="2" y="21" width="7" height="7" rx="1.25" />
      <rect x="11.5" y="21" width="7" height="7" rx="1.25" />
    </g>
    <rect className="brand-logo-signal" x="21" y="21" width="7" height="7" rx="1.25" />
  </svg>;
}

export function BrandLockup({subtitle,className=''}:{subtitle?:string;className?:string}) {
  return <span className={`brand-lockup ${className}`.trim()}><BrandMark/><span className="brand-lockup-copy"><strong>Content Work OS</strong>{subtitle&&<small>{subtitle}</small>}</span></span>;
}
