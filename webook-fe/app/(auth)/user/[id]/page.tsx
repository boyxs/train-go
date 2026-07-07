import UserDetailPage from '@/views/user/detail';

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <UserDetailPage userId={id} />;
}
