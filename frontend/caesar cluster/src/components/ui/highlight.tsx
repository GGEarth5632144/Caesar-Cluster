import { highlightParts } from "@/lib/search";
import { cn } from "@/lib/utils";

interface HighlightProps {
  text: string;
  terms: string[];
  className?: string;
}

/**
 * ทำตัวเน้นให้ส่วนของข้อความที่ตรงกับคำค้น
 * ใช้ในตารางที่กรองด้วย usePageSearch เพื่อให้เห็นว่าแถวนี้ถูกเลือกมาเพราะอะไร
 */
export function Highlight({ text, terms, className }: HighlightProps) {
  const parts = highlightParts(text, terms);

  // ไม่มีคำไหนตรงเลย — คืนข้อความเปล่าๆ ไม่ต้องห่อ span ให้ DOM หนักขึ้นโดยเปล่าประโยชน์
  if (parts.length === 1 && !parts[0].hit) return <>{text}</>;

  return (
    <>
      {parts.map((part, index) =>
        part.hit ? (
          <mark
            key={index}
            className={cn("rounded-[3px] bg-[#F08B51]/35 px-0.5 text-inherit", className)}
          >
            {part.text}
          </mark>
        ) : (
          <span key={index}>{part.text}</span>
        ),
      )}
    </>
  );
}

export default Highlight;
