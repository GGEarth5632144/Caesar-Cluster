import { Navigate } from "react-router-dom";
import { PATHS } from "@/config/routes";

export default function Createservice() {
  return <Navigate to={`/${PATHS.services}`} replace />;
}
