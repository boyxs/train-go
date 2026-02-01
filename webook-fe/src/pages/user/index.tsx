import type { Metadata } from 'next';
import React from 'react';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

export const metadata: Metadata = {
  title: '小微书',
  description: '你的第一个 Web 应用',
};

const Appw = () => {
  return <div>hello</div>;
};

export default Appw;
