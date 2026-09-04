// ภาพประกอบฝั่งขวาของหน้า Login/Register — วาดด้วย inline SVG ล้วน
// เลยไม่ต้องโหลดไฟล์ภาพเพิ่ม และปรับสีตามธีมแบรนด์ได้ตรงๆ
const NODE_DOTS = [
  { i: 0, j: 0 },
  { i: 1, j: 0 },
  { i: 0, j: 1 },
  { i: -1, j: 0 },
  { i: 0, j: -1 },
  { i: 1, j: 1 },
];

// แปลงพิกัดกริดแบบ isometric -> พิกัดจริงบนหน้าจอ
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

// แผ่น cluster หนึ่งชั้น: หน้าบนเป็นสี่เหลี่ยมข้าวหลามตัด + ด้านข้างซ้าย/ขวาให้ดูหนา
function Slab({ cy, top, left, right, delay }: SlabProps) {
  const halfH = 65;
  const depth = 18;

  return (
    <g className="cc-hero-float" style={{ animationDelay: delay }}>
      <polygon
        points={`200,${cy + halfH} 70,${cy} 70,${cy + depth} 200,${cy + halfH + depth}`}
        fill={left}
      />
      <polygon
        points={`200,${cy + halfH} 330,${cy} 330,${cy + depth} 200,${cy + halfH + depth}`}
        fill={right}
      />
      <polygon
        points={`200,${cy - halfH} 330,${cy} 200,${cy + halfH} 70,${cy}`}
        fill={top}
      />
      <polygon
        points={`200,${cy - halfH} 330,${cy} 200,${cy + halfH} 70,${cy}`}
        fill="none"
        stroke="#FFF8E8"
        strokeOpacity="0.35"
      />
    </g>
  );
}

export default function AuthHeroArt() {
  return (
    <div className="relative flex h-full w-full items-center justify-center overflow-hidden bg-[#FFF8E8]">
      {/* แสงอุ่นๆ หลังภาพ ให้พื้นครีมไม่โล่งจนเกินไป */}
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(60%_55%_at_50%_42%,rgba(240,139,81,0.28),transparent_70%)]" />
      {/* กริดจุดจางๆ */}
      <div
        className="pointer-events-none absolute inset-0 opacity-40"
        style={{
          backgroundImage:
            "radial-gradient(rgba(187,102,83,0.35) 1px, transparent 1px)",
          backgroundSize: "26px 26px",
        }}
      />

      <div className="relative flex w-full max-w-lg flex-col items-center px-10">
        <svg
          viewBox="0 0 400 400"
          className="w-full max-w-md"
          role="img"
          aria-label="ภาพประกอบคลัสเตอร์ของ Caesar Cluster"
        >
          {/* เงาใต้กองแผ่น */}
          <ellipse
            cx="200"
            cy="352"
            rx="118"
            ry="26"
            fill="#BB6653"
            opacity="0.18"
          />

          {/* เส้นประเชื่อมแต่ละชั้นเข้าด้วยกัน */}
          <g stroke="#BB6653" strokeOpacity="0.45" strokeDasharray="5 6">
            <line x1="70" y1="128" x2="70" y2="290" />
            <line x1="330" y1="128" x2="330" y2="290" />
          </g>

          {/* จุดข้อมูลวิ่งขึ้นตามเส้นเชื่อม */}
          <g fill="#F08B51">
            <circle cx="70" cy="290" r="4" className="cc-hero-packet" />
            <circle
              cx="330"
              cy="290"
              r="4"
              className="cc-hero-packet"
              style={{ animationDelay: "-1.4s" }}
            />
          </g>

          <Slab cy={290} top="#8E4A3D" left="#6E382E" right="#7B3F34" delay="-0.8s" />
          <Slab cy={200} top="#BB6653" left="#93503F" right="#A45947" delay="-0.4s" />
          <Slab cy={110} top="#F08B51" left="#C96C3C" right="#DC7B47" delay="0s" />

          {/* โหนดบนแผ่นชั้นบนสุด — กะพริบไล่กันเหมือน pod ที่กำลังรัน */}
          <g>
            {NODE_DOTS.map(({ i, j }, index) => {
              const { x, y } = isoPoint(i, j, 200, 110);
              return (
                <g key={`${i}-${j}`}>
                  <circle
                    cx={x}
                    cy={y}
                    r="9"
                    fill="#FFF8E8"
                    className="cc-hero-node"
                    style={{ animationDelay: `${index * 0.28}s` }}
                  />
                  <circle cx={x} cy={y} r="3.5" fill="#BB6653" />
                </g>
              );
            })}
          </g>
        </svg>

        <p className="mt-8 text-center text-2xl font-semibold text-[#211a14]">
          Cloud for CPE Students
        </p>
        <p className="mt-2 text-center text-sm text-[#211a14]/60">
          สร้าง จัดการ และติดตามเซอร์วิสของคุณบนคลัสเตอร์เดียว
        </p>
      </div>
    </div>
  );
}
