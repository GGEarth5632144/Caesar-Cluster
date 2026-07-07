import { useState, useEffect } from 'react';
import { userApi, type User } from '@/api/userApi';

export default function Users() {
  const [users, setUsers] = useState<User[]>([]);

  useEffect(() => {
    // เรียกใช้ฟังก์ชันจากโฟลเดอร์ api
    const fetchUsers = async () => {
      try {
        const data = await userApi.getAllUsers();
        setUsers(data);
      } catch (error) {
        console.error("ดึงข้อมูลไม่สำเร็จ", error);
      }
    };

    fetchUsers();
  }, []);

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold mb-4">รายชื่อผู้ใช้งาน</h1>
      <ul>
        {users.map((user) => (
          <li key={user.id}>{user.name} - {user.email}</li>
        ))}
      </ul>
    </div>
  );
}

export { Users };