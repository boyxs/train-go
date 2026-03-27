import ArticleEditPage from '@/views/article/edit';

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <ArticleEditPage articleId={id} />;
}
