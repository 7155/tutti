import {
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent
} from "react";

/**
 * Two-dimensional Tutti preference pad. The horizontal axis is speed and the
 * vertical axis is effect (top = 100). Pointer drags move the handle freely;
 * keyboard and assistive-tech control stays on the two visually hidden
 * Radix sliders the popover renders next to this pad.
 *
 * The axis readouts stay hidden until the pad is hovered or the handle is
 * dragged, keeping the pad itself free of permanent chrome. Visual language
 * mirrors the intensity dot-grid explorations: a faint gradient (hue along
 * speed, brightness along effect) under a gravity dot field, with the Tutti
 * IP handle image.
 */

/** Handle image box in px (same as the former effect-slider thumb). */
const HANDLE_SIZE = 40;
/** Handle travel inset so the handle never crosses the pad edge. */
const HANDLE_INSET = HANDLE_SIZE / 2 + 4;

/**
 * Dot-field coordinate space. The pad is 4:3, so the lattice uses a 100x75
 * view box: one unit measures the same physical length on both axes, which
 * keeps dots round and the gravity well isotropic.
 */
const FIELD_WIDTH = 100;
const FIELD_HEIGHT = 75;
/** Dot lattice resolution; the pitch is equal on both axes. */
const DOT_COLS = 12;
const DOT_ROWS = 9;
/** Gravity-well sigma in percent units: how far the handle's pull reaches. */
const GRAVITY_SIGMA = 24;
/** Edge fade band in percent units, measured from the pad center. Dots fade
 * out between the start and end of this band so the lattice dissolves before
 * it reaches the pad's rounded corners. */
const EDGE_FADE_START = 22;
const EDGE_FADE_END = 50;

function clampRatio(value: number): number {
  return Math.min(1, Math.max(0, value));
}

interface DotSpec {
  cx: number;
  cy: number;
  r: number;
  opacity: number;
}

/**
 * Builds the gravity dot field: every dot swells and brightens as the handle
 * approaches (gaussian falloff), while a radial vignette fades dots out
 * towards the pad edges.
 */
function buildDotSpecs(handleX: number, handleY: number): DotSpec[] {
  const specs: DotSpec[] = [];
  const stepX = FIELD_WIDTH / DOT_COLS;
  const stepY = FIELD_HEIGHT / DOT_ROWS;
  for (let row = 0; row < DOT_ROWS; row += 1) {
    for (let col = 0; col < DOT_COLS; col += 1) {
      const cx = (col + 0.5) * stepX;
      const cy = (row + 0.5) * stepY;
      const handleDistance = Math.hypot(cx - handleX, cy - handleY);
      const pull = Math.exp(-((handleDistance / GRAVITY_SIGMA) ** 2));
      const centerDistance = Math.hypot(
        cx - FIELD_WIDTH / 2,
        cy - FIELD_HEIGHT / 2
      );
      const edgeFade = clampRatio(
        (EDGE_FADE_END - centerDistance) / (EDGE_FADE_END - EDGE_FADE_START)
      );
      specs.push({
        cx,
        cy,
        r: 0.5 + 1.4 * pull,
        opacity: edgeFade * (0.5 + 0.5 * pull)
      });
    }
  }
  return specs;
}

export function TuttiBudgetPad({
  effect,
  speed,
  effectLabel,
  speedLabel,
  handleUrl,
  onChange
}: {
  /** Current draft effect (0-100), mapped to the vertical axis. */
  effect: number;
  /** Current draft speed (0-100), mapped to the horizontal axis. */
  speed: number;
  effectLabel: string;
  speedLabel: string;
  /** Tutti IP handle image for the current effect tier. */
  handleUrl: string;
  onChange(next: { effect: number; speed: number }): void;
}): React.JSX.Element {
  const padRef = useRef<HTMLDivElement>(null);
  const [dragging, setDragging] = useState(false);

  const applyPointer = (event: ReactPointerEvent<HTMLDivElement>): void => {
    const pad = padRef.current;
    if (!pad) {
      return;
    }
    const rect = pad.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) {
      return;
    }
    const xRatio = clampRatio((event.clientX - rect.left) / rect.width);
    const yRatio = clampRatio((event.clientY - rect.top) / rect.height);
    onChange({
      speed: Math.round(xRatio * 100),
      effect: Math.round((1 - yRatio) * 100)
    });
  };

  // Handle position in field space: x follows speed, y follows effect with
  // the axis flipped so 100 sits at the top.
  const handleX = speed;
  const handleY = ((100 - effect) / 100) * FIELD_HEIGHT;
  const dots = buildDotSpecs(handleX, handleY);

  return (
    <div
      ref={padRef}
      data-agent-tutti-preference-pad="true"
      data-dragging={dragging ? "true" : undefined}
      className="group/pad relative aspect-[4/3] w-full cursor-crosshair touch-none overflow-hidden rounded-[12px] select-none"
      style={
        {
          backgroundImage:
            "linear-gradient(180deg, color-mix(in srgb, var(--white-stationary) 38%, transparent) 0%, transparent 55%), linear-gradient(100deg, color-mix(in srgb, var(--state-success) 60%, transparent) 0%, color-mix(in srgb, var(--accent-codex) 60%, transparent) 55%, color-mix(in srgb, var(--tutti-purple) 60%, transparent) 100%)"
        } as CSSProperties
      }
      onPointerDown={(event) => {
        // jsdom and older runtimes lack pointer capture; without it the drag
        // still tracks while the pointer stays over the pad.
        if (typeof event.currentTarget.setPointerCapture === "function") {
          event.currentTarget.setPointerCapture(event.pointerId);
        }
        setDragging(true);
        applyPointer(event);
      }}
      onPointerMove={(event) => {
        if ((event.buttons & 1) === 1) {
          applyPointer(event);
        }
      }}
      onPointerUp={() => setDragging(false)}
      onPointerCancel={() => setDragging(false)}
    >
      {/* Gravity dot field: dots swell towards the handle and dissolve
          towards the pad edges. */}
      <svg
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 h-full w-full"
        viewBox={`0 0 ${FIELD_WIDTH} ${FIELD_HEIGHT}`}
        preserveAspectRatio="none"
      >
        {dots.map((dot) => (
          <circle
            key={`${dot.cx}:${dot.cy}`}
            cx={dot.cx}
            cy={dot.cy}
            r={dot.r}
            fill="var(--white-stationary)"
            opacity={dot.opacity}
          />
        ))}
      </svg>
      {/* Axis readouts: only visible while hovering the pad or dragging the
          handle, so the resting pad carries no permanent chrome. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-2.5 top-2 flex items-start justify-between text-[11px] text-[var(--white-stationary)] opacity-0 transition-opacity duration-150 group-hover/pad:opacity-100 group-data-[dragging=true]/pad:opacity-100"
      >
        <span data-agent-tutti-pad-readout="effect">
          {effectLabel} <span className="tabular-nums">{effect}</span>
        </span>
      </div>
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-2.5 bottom-2 flex items-end justify-end text-[11px] text-[var(--white-stationary)] opacity-0 transition-opacity duration-150 group-hover/pad:opacity-100 group-data-[dragging=true]/pad:opacity-100"
      >
        <span data-agent-tutti-pad-readout="speed">
          {speedLabel} <span className="tabular-nums">{speed}</span>
        </span>
      </div>
      <div
        aria-hidden="true"
        data-agent-tutti-preference-handle="true"
        className="pointer-events-none absolute rounded-full bg-transparent bg-contain bg-center bg-no-repeat group-has-[[data-slot=slider-thumb]:focus-visible]:ring-2 group-has-[[data-slot=slider-thumb]:focus-visible]:ring-[var(--border-focus)]"
        style={
          {
            width: HANDLE_SIZE,
            height: HANDLE_SIZE,
            backgroundImage: `url("${handleUrl}")`,
            left: `calc(${HANDLE_INSET}px + (100% - ${HANDLE_INSET * 2}px) * ${speed / 100})`,
            top: `calc(${HANDLE_INSET}px + (100% - ${HANDLE_INSET * 2}px) * ${(100 - effect) / 100})`,
            transform: "translate(-50%, -50%)"
          } as CSSProperties
        }
      />
    </div>
  );
}
