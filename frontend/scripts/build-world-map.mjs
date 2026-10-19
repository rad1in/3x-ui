// Regenerates src/assets/world-map.json from Natural Earth 110m (public domain) via the
// world-atlas TopoJSON. Usage: node scripts/build-world-map.mjs countries-110m.json iso_3166-1.json
import { readFileSync, writeFileSync } from 'node:fs';

const [topoFile, isoFile] = process.argv.slice(2);
const topo = JSON.parse(readFileSync(topoFile, 'utf8'));
const iso = JSON.parse(readFileSync(isoFile, 'utf8'))['3166-1'];
const alpha2 = new Map(iso.map((c) => [String(Number(c.numeric)), c.alpha_2]));
// Natural Earth ids the ISO table does not carry.
const nameFallback = { Kosovo: 'XK' };

const { scale, translate } = topo.transform;
const arcs = topo.arcs.map((arc) => {
  let x = 0;
  let y = 0;
  return arc.map(([dx, dy]) => {
    x += dx;
    y += dy;
    return [x * scale[0] + translate[0], y * scale[1] + translate[1]];
  });
});

function ring(indexes) {
  const pts = [];
  for (const i of indexes) {
    const a = i < 0 ? arcs[~i].slice().reverse() : arcs[i];
    pts.push(...(pts.length ? a.slice(1) : a));
  }
  return pts;
}

// Rings that cross the antimeridian (Russia, Fiji) come unsplit: unwrap them, then cut at
// 180 degrees and shift the eastern part back so no edge streaks across the map.
function clipX(pts, keep, edge) {
  const out = [];
  for (let i = 0; i < pts.length; i++) {
    const a = pts[i];
    const b = pts[(i + 1) % pts.length];
    const ain = keep(a[0]);
    const bin = keep(b[0]);
    if (ain) out.push(a);
    if (ain !== bin) {
      const t = (edge - a[0]) / (b[0] - a[0]);
      out.push([edge, a[1] + t * (b[1] - a[1])]);
    }
  }
  return out;
}

function splitAtAntimeridian(pts) {
  let offset = 0;
  let jumped = false;
  const un = pts.map((p, i) => {
    if (i > 0) {
      const d = p[0] - pts[i - 1][0];
      if (d > 180) {
        offset -= 360;
        jumped = true;
      } else if (d < -180) {
        offset += 360;
        jumped = true;
      }
    }
    return [p[0] + offset, p[1]];
  });
  if (!jumped) return [pts];
  const hi = Math.max(...un.map((p) => p[0]));
  const lo = Math.min(...un.map((p) => p[0]));
  const parts = [];
  const inRange = clipX(
    clipX(un, (x) => x <= 180, 180),
    (x) => x >= -180,
    -180,
  );
  if (inRange.length > 2) parts.push(inRange);
  if (hi > 180) {
    const east = clipX(un, (x) => x >= 180, 180).map(([x, y]) => [x - 360, y]);
    if (east.length > 2) parts.push(east);
  }
  if (lo < -180) {
    const w = clipX(un, (x) => x <= -180, -180).map(([x, y]) => [x + 360, y]);
    if (w.length > 2) parts.push(w);
  }
  return parts;
}

// Natural Earth projection (Savric 2011).
function project([lon, lat]) {
  const l = (lon * Math.PI) / 180;
  const p = (lat * Math.PI) / 180;
  const p2 = p * p;
  const p4 = p2 * p2;
  const x = l * (0.8707 - 0.131979 * p2 + p4 * (-0.013791 + p4 * (0.003971 * p2 - 0.001529 * p4)));
  const y = p * (1.007226 + p2 * (0.015085 + p4 * (-0.044475 + 0.028874 * p2 - 0.005916 * p4)));
  return [x, -y];
}

const geoms = topo.objects.countries.geometries.filter((g) => g.properties.name !== 'Antarctica');
const projected = geoms.map((g) => {
  const polys = g.type === 'Polygon' ? [g.arcs] : g.arcs;
  return {
    g,
    rings: polys.flatMap((rs) =>
      rs.flatMap((r) => splitAtAntimeridian(ring(r)).map((part) => part.map(project))),
    ),
  };
});

let minX = Infinity,
  minY = Infinity,
  maxX = -Infinity,
  maxY = -Infinity;
for (const { rings } of projected)
  for (const r of rings)
    for (const [x, y] of r) {
      minX = Math.min(minX, x);
      maxX = Math.max(maxX, x);
      minY = Math.min(minY, y);
      maxY = Math.max(maxY, y);
    }
const k = 1000 / (maxX - minX);
const width = 1000;
const height = Math.ceil((maxY - minY) * k);

const countries = [];
for (const { g, rings } of projected) {
  const code = alpha2.get(String(Number(g.id))) ?? nameFallback[g.properties.name] ?? null;
  let d = '';
  for (const r of rings) {
    let px = 0;
    let py = 0;
    let first = true;
    for (const [x, y] of r) {
      const sx = Math.round((x - minX) * k * 10) / 10;
      const sy = Math.round((y - minY) * k * 10) / 10;
      if (first) {
        d += `M${sx} ${sy}`;
      } else {
        const dx = Math.round((sx - px) * 10) / 10;
        const dy = Math.round((sy - py) * 10) / 10;
        if (dx === 0 && dy === 0) continue;
        d += `l${dx} ${dy}`;
      }
      px = sx;
      py = sy;
      first = false;
    }
    d += 'z';
  }
  countries.push({ code, name: g.properties.name, d });
}
writeFileSync(
  new URL('../src/assets/world-map.json', import.meta.url),
  JSON.stringify({ width, height, countries }),
);
console.log(
  countries.length,
  'countries',
  countries.filter((c) => !c.code).map((c) => c.name),
);
