import { Spin } from 'antd';
import React from 'react';

interface LoadingProps {
  tip?: string;
}

export const Loading: React.FC<LoadingProps> = ({ tip = '加载中...' }) => {
  return (
    <div className='flex justify-center items-center min-h-[200px]'>
      <Spin tip={tip} size='large'>
        <div className='p-12' />
      </Spin>
    </div>
  );
};
