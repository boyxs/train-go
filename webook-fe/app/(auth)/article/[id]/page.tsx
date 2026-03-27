import ArticleReadPage from '@/views/article/read';

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <ArticleReadPage articleId={id} />;
}
