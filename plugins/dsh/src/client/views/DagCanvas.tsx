import { useMemo, useRef, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import type { PacketNode, Translate } from '../types.ts';

const NODE_W = 244;
const NODE_H = 96;
const COL_GAP = 84;
const ROW_GAP = 28;
const PAD_X = 32;
const PAD_Y = 32;

interface PositionedNode {
  packet: PacketNode;
  col: number;
  row: number;
  x: number;
  y: number;
  deps: string[];
}

interface DagLayout {
  nodes: PositionedNode[];
  byId: Map<string, PositionedNode>;
  edges: Array<{ from: PositionedNode; to: PositionedNode; active: boolean }>;
  width: number;
  height: number;
}

function packetDeps(packet: PacketNode): string[] {
  const raw = packet.dependsOn ?? packet.depends_on;
  return Array.isArray(raw) ? raw.filter(v => typeof v === 'string') : [];
}

/**
 * Computes a Sugiyama-style topological layered DAG layout with barycenter
 * row ordering and zero node pagination — every packet is rendered at once.
 */
export function computeDagLayout(packets: PacketNode[], waves?: string[][]): DagLayout {
  const map = new Map<string, PacketNode>();
  for (const p of packets) map.set(p.id, p);

  const layerById = new Map<string, number>();
  if (Array.isArray(waves) && waves.length > 0) {
    waves.forEach((ids, idx) => {
      for (const id of ids) {
        if (map.has(id)) layerById.set(id, idx);
      }
    });
  }

  const visiting = new Set<string>();
  const resolveLayer = (id: string): number => {
    const known = layerById.get(id);
    if (known !== undefined) return known;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const p = map.get(id);
    if (!p) return 0;
    if (typeof p.wave === 'number' && Number.isInteger(p.wave) && p.wave >= 0) {
      layerById.set(id, p.wave);
      visiting.delete(id);
      return p.wave;
    }
    const deps = packetDeps(p).filter(d => map.has(d));
    const layer = deps.length === 0 ? 0 : Math.max(...deps.map(resolveLayer)) + 1;
    layerById.set(id, layer);
    visiting.delete(id);
    return layer;
  };

  for (const p of packets) resolveLayer(p.id);

  const columns = new Map<number, PacketNode[]>();
  for (const p of packets) {
    const col = layerById.get(p.id) ?? 0;
    const list = columns.get(col) ?? [];
    list.push(p);
    columns.set(col, list);
  }

  const sortedCols = [...columns.keys()].sort((a, b) => a - b);
  const rowIndex = new Map<string, number>();

  // Barycenter ordering across layers to reduce edge crossings.
  for (const col of sortedCols) {
    const list = columns.get(col)!;
    list.sort((a, b) => {
      const bary = (p: PacketNode): number => {
        const deps = packetDeps(p).filter(d => rowIndex.has(d));
        if (deps.length === 0) return Number.MAX_SAFE_INTEGER;
        return deps.reduce((sum, d) => sum + (rowIndex.get(d) ?? 0), 0) / deps.length;
      };
      const diff = bary(a) - bary(b);
      return diff !== 0 ? diff : a.id.localeCompare(b.id);
    });
    list.forEach((p, idx) => rowIndex.set(p.id, idx));
  }

  const nodes: PositionedNode[] = [];
  const byId = new Map<string, PositionedNode>();
  let maxX = PAD_X * 2 + NODE_W;
  let maxY = PAD_Y * 2 + NODE_H;

  sortedCols.forEach((col, colPos) => {
    const list = columns.get(col)!;
    list.forEach((packet, row) => {
      const x = PAD_X + colPos * (NODE_W + COL_GAP);
      const y = PAD_Y + row * (NODE_H + ROW_GAP);
      const positioned: PositionedNode = {
        packet,
        col: colPos,
        row,
        x,
        y,
        deps: packetDeps(packet).filter(d => map.has(d)),
      };
      nodes.push(positioned);
      byId.set(packet.id, positioned);
      maxX = Math.max(maxX, x + NODE_W + PAD_X);
      maxY = Math.max(maxY, y + NODE_H + PAD_Y);
    });
  });

  const edges: Array<{ from: PositionedNode; to: PositionedNode; active: boolean }> = [];
  for (const to of nodes) {
    for (const depId of to.deps) {
      const from = byId.get(depId);
      if (!from) continue;
      const toStatus = (to.packet.status ?? '').toLowerCase();
      const active = ['running', 'executing', 'reviewing'].includes(toStatus);
      edges.push({ from, to, active });
    }
  }

  return { nodes, byId, edges, width: maxX, height: maxY };
}

function collectLineage(
  targetId: string,
  layout: DagLayout,
): { nodes: Set<string>; edges: Set<string> } {
  const nodes = new Set<string>([targetId]);
  const edges = new Set<string>();

  // Upstream ancestors
  const upQueue = [targetId];
  while (upQueue.length > 0) {
    const curr = upQueue.shift()!;
    const node = layout.byId.get(curr);
    if (!node) continue;
    for (const dep of node.deps) {
      edges.add(`${dep}->${curr}`);
      if (!nodes.has(dep)) {
        nodes.add(dep);
        upQueue.push(dep);
      }
    }
  }

  // Downstream dependents
  const downQueue = [targetId];
  while (downQueue.length > 0) {
    const curr = downQueue.shift()!;
    for (const edge of layout.edges) {
      if (edge.from.packet.id === curr) {
        edges.add(`${curr}->${edge.to.packet.id}`);
        if (!nodes.has(edge.to.packet.id)) {
          nodes.add(edge.to.packet.id);
          downQueue.push(edge.to.packet.id);
        }
      }
    }
  }

  return { nodes, edges };
}

function dotTone(status?: string): string {
  const s = (status ?? '').toLowerCase();
  if (['pass', 'passed', 'done', 'completed', 'ok', 'approved'].includes(s)) return styles.dotPass;
  if (['fail', 'failed', 'blocked', 'rejected', 'error'].includes(s)) return styles.dotFail;
  if (['needs_polish', 'warn', 'returned', 'advisory'].includes(s)) return styles.dotWarn;
  if (['running', 'executing', 'reviewing'].includes(s)) return styles.dotRun;
  return '';
}

function statusPillClass(status?: string): string {
  const s = (status ?? '').toLowerCase();
  if (['done', 'completed', 'passed'].includes(s)) return styles.pillSuccess;
  if (['running', 'executing', 'reviewing'].includes(s)) return styles.pillRunning;
  if (['failed', 'blocked', 'escalated'].includes(s)) return styles.pillDanger;
  if (['stopped', 'awaiting_approval'].includes(s)) return styles.pillWarn;
  return '';
}

export function DagCanvas({
  packets,
  waves,
  selectedPacketId,
  onSelectPacket,
  t,
}: {
  packets: PacketNode[];
  waves?: string[][];
  selectedPacketId: string | null;
  onSelectPacket: (packetId: string) => void;
  t: Translate;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [scale, setScale] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const dragRef = useRef<{ active: boolean; startX: number; startY: number; origX: number; origY: number }>({
    active: false,
    startX: 0,
    startY: 0,
    origX: 0,
    origY: 0,
  });

  const layout = useMemo(() => computeDagLayout(packets, waves), [packets, waves]);
  const lineage = useMemo(
    () => (hoveredId ? collectLineage(hoveredId, layout) : null),
    [hoveredId, layout],
  );

  const clampScale = (next: number) => Math.max(0.35, Math.min(2.2, Number(next.toFixed(2))));

  const handleWheel: React.WheelEventHandler<HTMLDivElement> = event => {
    event.preventDefault();
    const rect = containerRef.current?.getBoundingClientRect();
    const factor = event.deltaY < 0 ? 1.1 : 0.9;
    const nextScale = clampScale(scale * factor);
    if (!rect) {
      setScale(nextScale);
      return;
    }
    const cx = event.clientX - rect.left;
    const cy = event.clientY - rect.top;
    const ratio = nextScale / scale;
    setPan(prev => ({
      x: cx - (cx - prev.x) * ratio,
      y: cy - (cy - prev.y) * ratio,
    }));
    setScale(nextScale);
  };

  const handlePointerDown: React.PointerEventHandler<HTMLDivElement> = event => {
    if ((event.target as HTMLElement).closest('button')) return;
    dragRef.current = {
      active: true,
      startX: event.clientX,
      startY: event.clientY,
      origX: pan.x,
      origY: pan.y,
    };
  };

  const handlePointerMove: React.PointerEventHandler<HTMLDivElement> = event => {
    if (!dragRef.current.active) return;
    setPan({
      x: dragRef.current.origX + (event.clientX - dragRef.current.startX),
      y: dragRef.current.origY + (event.clientY - dragRef.current.startY),
    });
  };

  const handlePointerUp = () => {
    dragRef.current.active = false;
  };

  const handleFit = () => {
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect || layout.width === 0 || layout.height === 0) {
      setScale(1);
      setPan({ x: 0, y: 0 });
      return;
    }
    const sx = (rect.width - 32) / layout.width;
    const sy = (rect.height - 48) / layout.height;
    const next = clampScale(Math.min(1, sx, sy));
    setScale(next);
    setPan({
      x: Math.max(12, (rect.width - layout.width * next) / 2),
      y: Math.max(12, (rect.height - layout.height * next) / 2),
    });
  };

  const miniScaleX = 148 / Math.max(layout.width, 1);
  const miniScaleY = 84 / Math.max(layout.height, 1);
  const miniScale = Math.min(miniScaleX, miniScaleY);

  return (
    <div
      ref={containerRef}
      className={styles.dagContainer}
      data-testid="flywheel-dag-canvas"
      onWheel={handleWheel}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerLeave={handlePointerUp}
    >
      {/* Floating glass toolbar */}
      <div className={styles.dagToolbar}>
        <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => setScale(s => clampScale(s - 0.15))} title={t('exec.dag.zoomOut')}>
          −
        </button>
        <span className={styles.dagZoomLabel} data-testid="dag-zoom-label">
          {Math.round(scale * 100)}%
        </span>
        <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => setScale(s => clampScale(s + 0.15))} title={t('exec.dag.zoomIn')}>
          +
        </button>
        <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={handleFit}>
          {t('exec.dag.fit')}
        </button>
        <button
          type="button"
          className={`${styles.btn} ${styles.btnSm}`}
          onClick={() => {
            setScale(1);
            setPan({ x: 0, y: 0 });
          }}
        >
          {t('exec.dag.reset')}
        </button>
      </div>

      {/* Transformed viewport */}
      <div
        className={styles.dagViewport}
        style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${scale})` }}
      >
        <svg className={styles.dagEdgesSvg} width={layout.width} height={layout.height}>
          <defs>
            <marker id="fw-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 1.5 L 9 5 L 0 8.5 z" fill="rgba(152,161,181,0.6)" />
            </marker>
            <marker id="fw-arrow-active" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 1.5 L 9 5 L 0 8.5 z" fill="#5b8cff" />
            </marker>
          </defs>
          {layout.edges.map(edge => {
            const x1 = edge.from.x + NODE_W;
            const y1 = edge.from.y + NODE_H / 2;
            const x2 = edge.to.x;
            const y2 = edge.to.y + NODE_H / 2;
            const dx = Math.max(36, (x2 - x1) * 0.45);
            const d = `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
            const key = `${edge.from.packet.id}->${edge.to.packet.id}`;
            const isHighlighted = lineage ? lineage.edges.has(key) : false;
            const isDim = lineage !== null && !isHighlighted;
            const cls = [
              styles.dagEdgePath,
              edge.active ? styles.dagEdgeActive : '',
              isHighlighted ? styles.dagEdgeHighlight : '',
              isDim ? styles.dagEdgeDim : '',
            ]
              .filter(Boolean)
              .join(' ');
            return (
              <path
                key={key}
                d={d}
                className={cls}
                markerEnd={edge.active || isHighlighted ? 'url(#fw-arrow-active)' : 'url(#fw-arrow)'}
              />
            );
          })}
        </svg>

        {layout.nodes.map(node => {
          const p = node.packet;
          const isRunning = ['running', 'executing', 'reviewing'].includes((p.status ?? '').toLowerCase());
          const isHighlighted = lineage ? lineage.nodes.has(p.id) : selectedPacketId === p.id;
          const isDim = lineage !== null && !lineage.nodes.has(p.id);
          const nodeClass = [
            styles.dagNode,
            isRunning ? styles.dagNodeRunning : '',
            isHighlighted ? styles.dagNodeHighlight : '',
            isDim ? styles.dagNodeDim : '',
          ]
            .filter(Boolean)
            .join(' ');
          return (
            <button
              key={p.id}
              type="button"
              className={nodeClass}
              style={{ left: node.x, top: node.y }}
              data-packet-id={p.id}
              data-lineage-dim={isDim ? 'true' : 'false'}
              onMouseEnter={() => setHoveredId(p.id)}
              onMouseLeave={() => setHoveredId(prev => (prev === p.id ? null : prev))}
              onClick={() => onSelectPacket(p.id)}
            >
              <div className={styles.packetCardTop}>
                <span className={styles.packetId}>{p.id}</span>
                <span className={`${styles.pill} ${statusPillClass(p.status)}`}>
                  {p.status ?? 'pending'}
                </span>
              </div>
              <div className={styles.packetGoal}>{p.goal || p.title || p.id}</div>
              <div className={styles.packetFooter}>
                <span>{p.route || p.profile || 'default'}</span>
                <span className={styles.microDots}>
                  <span className={styles.microDot} title={t('exec.dot.l1')}>
                    <span className={`${styles.dotCircle} ${dotTone(p.l1Status)}`} />
                    {t('exec.dot.l1')}
                  </span>
                  <span className={styles.microDot} title={t('exec.dot.ux')}>
                    <span className={`${styles.dotCircle} ${dotTone(p.uxStatus)}`} />
                    {t('exec.dot.ux')}
                  </span>
                  <span className={styles.microDot} title={t('exec.dot.review')}>
                    <span className={`${styles.dotCircle} ${dotTone(p.reviewStatus ?? p.reviewVerdict)}`} />
                    {t('exec.dot.review')}
                  </span>
                </span>
              </div>
            </button>
          );
        })}
      </div>

      {/* Minimap */}
      <div className={styles.dagMinimap} aria-label={t('exec.dag.minimap')}>
        <svg width="148" height="84">
          {layout.nodes.map(node => {
            const p = node.packet;
            const active = selectedPacketId === p.id || hoveredId === p.id;
            return (
              <rect
                key={p.id}
                x={node.x * miniScale}
                y={node.y * miniScale}
                width={Math.max(6, NODE_W * miniScale)}
                height={Math.max(4, NODE_H * miniScale)}
                rx="2"
                fill={active ? '#5b8cff' : 'rgba(152,161,181,0.55)'}
              />
            );
          })}
        </svg>
      </div>

      <div className={styles.dagHint}>{t('exec.dag.hint')}</div>
    </div>
  );
}
