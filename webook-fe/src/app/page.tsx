'use client';

import { Row } from 'antd';
import React from 'react';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import LoginForm from '../pages/user/login';
import RegisterForm from '../pages/user/register';

const Register = () => (
  <div>
    <Row justify='center' align={'middle'}>
      <RegisterForm></RegisterForm>
    </Row>
  </div>
);

const Login = () => (
  <div>
    <Row justify='center' align={'middle'}>
      <LoginForm></LoginForm>
    </Row>
  </div>
);

const App = () => {
  return (
    <BrowserRouter>
      <Routes>
        <Route index element={<Register />} />
        <Route path='register' element={<Register />} />
        <Route path='login' element={<Login />} />
      </Routes>
    </BrowserRouter>
  );
};

export default App;
