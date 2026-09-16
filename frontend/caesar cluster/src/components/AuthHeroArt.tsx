import { useRef, useState, type MouseEvent } from "react";

const NODE_DOTS = [
  { i: 0, j: 0 },
  { i: 1, j: 0 },
  { i: 0, j: 1 },
  { i: -1, j: 0 },
  { i: 0, j: -1 },
  { i: 1, j: 1 },
];

const isoPoint = (i: number, j: number, cx: number, cy: number) => ({
  x: cx + (i - j) * 38,
  y: cy + (i + j) * 19,
});

type SlabProps = {
  cy: number;
  top: string;
  left: string;
  right: string;
  delay: string;
};

// 1. Solid Slab โครงสร้างหลัก (ปรับเป็นสไตล์ Glassmorphism สีฟ้า-ขาว)
function Slab({ cy, top, left, right, delay }: SlabProps) {
  const halfH = 65;
  const depth = 18;
  return (
    <g className="cc-hero-float transition-transform duration-700" style={{ animationDelay: delay }}>
      <polygon points={`200,${cy + halfH} 70,${cy} 70,${cy + depth} 200,${cy + halfH + depth}`} fill={left} />
      <polygon points={`200,${cy + halfH} 330,${cy} 330,${cy + depth} 200,${cy + halfH + depth}`} fill={right} />
      <polygon points={`200,${cy - halfH} 330,${cy} 200,${cy + halfH} 70,${cy}`} fill={top} />
      {/* เส้นขอบบางๆ ให้ดูเป็นแผ่นกระจก */}
      <polygon points={`200,${cy - halfH} 330,${cy} 200,${cy + halfH} 70,${cy}`} fill="none" stroke="#FFFFFF" strokeWidth="1" opacity="0.6" />
    </g>
  );
}

// 2. Wireframe สำหรับ Reveal Highlight (เปลี่ยนเป็นเส้นเรืองแสงสี Cyan)
function Wireframe({ cy }: { cy: number }) {
  const halfH = 65;
  const depth = 18;
  return (
    <g fill="none" stroke="#06B6D4" strokeWidth="2" opacity="0.9" className="drop-shadow-[0_0_6px_rgba(6,182,212,0.8)]">
      <polygon points={`200,${cy + halfH} 70,${cy} 70,${cy + depth} 200,${cy + halfH + depth}`} />
      <polygon points={`200,${cy + halfH} 330,${cy} 330,${cy + depth} 200,${cy + halfH + depth}`} />
      <polygon points={`200,${cy - halfH} 330,${cy} 200,${cy + halfH} 70,${cy}`} />
    </g>
  );
}

// 3. Node Component ที่ประมวลผล Cursor Proximity & CSS Glow
const Node = ({ x, y, index, mouseX, mouseY }: { x: number; y: number; index: number; mouseX: number; mouseY: number }) => {
  const dx = x - mouseX;
  const dy = y - mouseY;
  const distance = Math.sqrt(dx * dx + dy * dy);
  
  const maxDistance = 110;
  const proximityFactor = Math.max(0, 1 - distance / maxDistance);
  
  const scale = 1 + proximityFactor * 0.4;
  const glowOpacity = proximityFactor * 0.9;

  return (
    <g 
      className="transition-transform duration-150 ease-out"
      style={{ transform: `translate(${x}px, ${y}px) scale(${scale})` }}
    >
      {/* รัศมีแสงสี Cyan ด้านนอก (Cursor Proximity Effect) */}
      <circle r="20" fill="#22D3EE" opacity={glowOpacity} className="blur-md" />
      
      {/* ตัว Node หลัก (Hover Glow Effect เปลี่ยนเป็นสีฟ้า Cloud) */}
      <circle
        r="9"
        fill="#FFFFFF"
        stroke="#06B6D4"
        strokeWidth="2"
        className="cursor-pointer transition-all duration-300 hover:fill=#E0F2FE drop-shadow-[0_0_4px_rgba(6,182,212,0.4)] hover:drop-shadow-[0_0_15px_rgba(6,182,212,1)]"
        style={{ animationDelay: `${index * 0.28}s` }}
      />
      <circle r="3.5" fill="#0284C7" />
    </g>
  );
};

export default function AuthHeroArt() {
  const containerRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  
  const [mousePos, setMousePos] = useState({ containerX: -1000, containerY: -1000, svgX: -1000, svgY: -1000 });
  const [isHovering, setIsHovering] = useState(false);

  const handleMouseMove = (e: MouseEvent<HTMLDivElement>) => {
    if (containerRef.current) {
      const rect = containerRef.current.getBoundingClientRect();
      const containerX = e.clientX - rect.left;
      const containerY = e.clientY - rect.top;
      
      if (svgRef.current) {
        const pt = svgRef.current.createSVGPoint();
        pt.x = e.clientX;
        pt.y = e.clientY;
        const CTM = svgRef.current.getScreenCTM();
        if (CTM) {
          const svgPt = pt.matrixTransform(CTM.inverse());
          setMousePos({ containerX, containerY, svgX: svgPt.x, svgY: svgPt.y });
        }
      }
    }
  };

  return (
    <div 
      ref={containerRef}
      className="relative flex h-full w-full items-center justify-center overflow-hidden bg-[#F8FAFC]"
      onMouseMove={handleMouseMove}
      onMouseEnter={() => setIsHovering(true)}
      onMouseLeave={() => {
        setIsHovering(false);
        setMousePos({ containerX: -1000, containerY: -1000, svgX: -1000, svgY: -1000 });
      }}
    >
      {/* แสงสว่างจางๆ พื้นหลังโทนสีฟ้า */}
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(60%_55%_at_50%_42%,rgba(56,189,248,0.12),transparent_70%)]" />
      
      {/* กริดจุดแบบล้ำยุค (Blueprint Grid) */}
      <div
        className="pointer-events-none absolute inset-0 opacity-50"
        style={{
          backgroundImage: "radial-gradient(rgba(148,163,184,0.4) 1px, transparent 1px)",
          backgroundSize: "28px 28px",
        }}
      />

      {/* 1. Spotlight Effect (ผสมกับแสงสี Cyan อ่อนๆ) */}
      <div 
        className="pointer-events-none absolute inset-0 z-10 transition-opacity duration-300"
        style={{
          background: `radial-gradient(450px circle at ${mousePos.containerX}px ${mousePos.containerY}px, rgba(255, 255, 255, 0.8), transparent 60%)`,
          opacity: isHovering ? 1 : 0,
          mixBlendMode: "overlay"
        }}
      />

      <div className="relative z-20 flex w-full max-w-lg flex-col items-center px-10">
        <svg
          ref={svgRef}
          viewBox="0 0 400 400"
          className="w-full max-w-md"
          role="img"
          aria-label="Caesar Cluster Futuristic Architecture"
        >
          <defs>
            {/* 4. Reveal Mask (กรอบแสงวูบวาบเมื่อเมาส์เข้าใกล้) */}
            <radialGradient id="reveal-mask" cx={mousePos.svgX} cy={mousePos.svgY} r="120" gradientUnits="userSpaceOnUse">
              <stop offset="0%" stopColor="white" stopOpacity="1" />
              <stop offset="100%" stopColor="white" stopOpacity="0" />
            </radialGradient>
          </defs>

          {/* เงาใต้กองแผ่น */}
          <ellipse cx="200" cy="352" rx="118" ry="26" fill="#94A3B8" opacity="0.25" className="blur-md" />

          {/* โครงสร้าง Slabs สไตล์ Crystal / Acrylic */}
          <Slab cy={290} top="#E2E8F0" left="#CBD5E1" right="#94A3B8" delay="-0.8s" />
          <Slab cy={200} top="#F1F5F9" left="#E2E8F0" right="#CBD5E1" delay="-0.4s" />
          <Slab cy={110} top="#FFFFFF" left="#F1F5F9" right="#E2E8F0" delay="0s" />

          {/* Reveal Highlight Layer - แสดงโครงลวดโฮโลแกรมเฉพาะที่เมาส์ส่องถึง */}
          <g mask="url(#reveal-mask)">
            <Wireframe cy={290} />
            <Wireframe cy={200} />
            <Wireframe cy={110} />
            
            <g stroke="#06B6D4" strokeWidth="2.5" strokeDasharray="4 4" className="drop-shadow-[0_0_5px_rgba(6,182,212,0.8)]">
              <line x1="70" y1="128" x2="70" y2="290" />
              <line x1="330" y1="128" x2="330" y2="290" />
            </g>
          </g>

          {/* เส้นประเชื่อมแต่ละชั้น (สีเทาฟ้า จางๆ) */}
          <g stroke="#94A3B8" strokeOpacity="0.6" strokeDasharray="5 6">
            <line x1="70" y1="128" x2="70" y2="290" />
            <line x1="330" y1="128" x2="330" y2="290" />
          </g>

          {/* จุดข้อมูลที่วิ่งขึ้นมา (Data Packets เรืองแสงสี Cyan) */}
          <g fill="#22D3EE" className="drop-shadow-[0_0_8px_rgba(34,211,238,1)]">
            <circle cx="70" cy="290" r="4.5" className="cc-hero-packet" />
            <circle cx="330" cy="290" r="4.5" className="cc-hero-packet" style={{ animationDelay: "-1.4s" }} />
          </g>

          {/* โหนดบนแผ่นชั้นบนสุด */}
          <g>
            {NODE_DOTS.map(({ i, j }, index) => {
              const { x, y } = isoPoint(i, j, 200, 110);
              return (
                <Node 
                  key={`${i}-${j}`} 
                  x={x} 
                  y={y} 
                  index={index} 
                  mouseX={mousePos.svgX} 
                  mouseY={mousePos.svgY} 
                />
              );
            })}
          </g>
        </svg>

        {/* ปรับสี Text ให้เข้ากับธีมใหม่ */}
        <p className="mt-8 text-center text-2xl font-bold tracking-wide text=#0F172A" >
          Cloud for <span className="text-[#0284C7]">CPE</span> Students
        </p>
      </div>
    </div>
  );
}